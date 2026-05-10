package images

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	renderpkg "github.com/oarkflow/interpreter/pkg/render"
	_ "golang.org/x/image/webp"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"image_load": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				img, errObj := resolveImageInput(env, args[0])
				if errObj != nil {
					return errObj
				}
				return img
			},
		},
		"image_resize": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 3 || len(args) > 4 {
					return object.NewError("wrong number of arguments. got=%d, want=3 or 4", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				width, errObj := asInt(args[1], "width")
				if errObj != nil {
					return errObj
				}
				height, errObj := asInt(args[2], "height")
				if errObj != nil {
					return errObj
				}
				opts, errObj := optionalOpts(args, 3)
				if errObj != nil {
					return errObj
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				return buildDerivedImageValue(img, imaging.Resize(src, int(width), int(height), imageFilterFromOpts(opts)), optString(opts, "format"))
			},
		},
		"image_crop": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 5 {
					return object.NewError("wrong number of arguments. got=%d, want=5", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				x, errObj := asInt(args[1], "x")
				if errObj != nil {
					return errObj
				}
				y, errObj := asInt(args[2], "y")
				if errObj != nil {
					return errObj
				}
				width, errObj := asInt(args[3], "width")
				if errObj != nil {
					return errObj
				}
				height, errObj := asInt(args[4], "height")
				if errObj != nil {
					return errObj
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				rect := image.Rect(int(x), int(y), int(x+width), int(y+height)).Intersect(src.Bounds())
				if rect.Empty() {
					return object.NewError("crop rectangle is outside image bounds")
				}
				return buildDerivedImageValue(img, imaging.Crop(src, rect), "")
			},
		},
		"image_rotate": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				degrees, ok := numberValue(args[1])
				if !ok {
					return object.NewError("argument `degrees` must be numeric")
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				return buildDerivedImageValue(img, imaging.Rotate(src, degrees, color.Transparent), "")
			},
		},
		"image_convert": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("wrong number of arguments. got=%d, want=2", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				format, errObj := asString(args[1], "format")
				if errObj != nil {
					return errObj
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				return buildDerivedImageValue(img, src, format)
			},
		},
		"image_info": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("wrong number of arguments. got=%d, want=1", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				pairs := map[object.HashKey]object.HashPair{}
				for key, value := range map[string]object.Object{
					"name":   &object.String{Value: img.Name},
					"mime":   &object.String{Value: img.MIME},
					"format": &object.String{Value: img.Format},
					"width":  &object.Integer{Value: img.Width},
					"height": &object.Integer{Value: img.Height},
					"size":   &object.Integer{Value: int64(len(img.Data))},
				} {
					k := &object.String{Value: key}
					pairs[k.HashKey()] = object.HashPair{Key: k, Value: value}
				}
				return &object.Hash{Pairs: pairs}
			},
		},
		"image_render": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				return &object.RenderArtifact{
					Kind:      "image",
					SourceTyp: "data",
					Source:    string(img.Data),
					Name:      img.Name,
					MIME:      img.MIME,
					Width:     img.Width,
					Height:    img.Height,
				}
			},
		},
	})
}

func resolveImageInput(env *object.Environment, obj object.Object) (*object.ImageValue, object.Object) {
	switch v := obj.(type) {
	case *object.ImageValue:
		if _, errObj := ensureDecodedImage(v); errObj != nil {
			return nil, errObj
		}
		return v, nil
	case *object.FileValue:
		return imageFromBytes(v.Data, v.Name, v.MIME)
	case *object.RenderArtifact:
		res, err := renderpkg.Resolve(context.Background(), env, v)
		if err != nil {
			return nil, object.NewError("image_load: %v", err)
		}
		return imageFromBytes(res.Bytes, res.Name, res.MIME)
	case *object.String:
		data, err := osReadFile(v.Value)
		if err != nil {
			return nil, object.NewError("image_load: %v", err)
		}
		return imageFromBytes(data, filepath.Base(v.Value), http.DetectContentType(data))
	default:
		return nil, object.NewError("argument `source` must be IMAGE_VALUE, FILE_VALUE, render artifact, or STRING, got %s", obj.Type())
	}
}

func imageFromBytes(data []byte, name, mimeType string) (*object.ImageValue, object.Object) {
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, object.NewError("image decode failed: %v", err)
	}
	img := &object.ImageValue{Name: name, Data: append([]byte(nil), data...), MIME: firstNonEmpty(mimeType, mimeForImageFormat(format)), Format: normalizeImageFormat(format), Image: src}
	bounds := src.Bounds()
	img.Width = int64(bounds.Dx())
	img.Height = int64(bounds.Dy())
	return img, nil
}

func ensureDecodedImage(img *object.ImageValue) (image.Image, object.Object) {
	if img == nil {
		return nil, object.NewError("image is null")
	}
	if img.Image != nil {
		return img.Image, nil
	}
	src, format, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return nil, object.NewError("image decode failed: %v", err)
	}
	img.Image = src
	img.Format = firstNonEmpty(img.Format, normalizeImageFormat(format))
	if img.MIME == "" {
		img.MIME = mimeForImageFormat(format)
	}
	bounds := src.Bounds()
	img.Width = int64(bounds.Dx())
	img.Height = int64(bounds.Dy())
	return src, nil
}

func buildDerivedImageValue(parent *object.ImageValue, src image.Image, format string) object.Object {
	format = normalizeImageFormat(firstNonEmpty(format, parent.Format, "png"))
	var buf bytes.Buffer
	var err error
	switch format {
	case "jpg", "jpeg":
		err = jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90})
	case "gif":
		err = gif.Encode(&buf, src, nil)
	default:
		format = "png"
		err = png.Encode(&buf, src)
	}
	if err != nil {
		return object.NewError("image encode failed: %v", err)
	}
	data := buf.Bytes()
	img := &object.ImageValue{Name: parent.Name, Data: append([]byte(nil), data...), Format: format, MIME: mimeForImageFormat(format), Image: src}
	bounds := src.Bounds()
	img.Width = int64(bounds.Dx())
	img.Height = int64(bounds.Dy())
	return img
}

func imageFilterFromOpts(opts map[string]object.Object) imaging.ResampleFilter {
	switch strings.ToLower(optString(opts, "filter")) {
	case "nearest":
		return imaging.NearestNeighbor
	case "linear":
		return imaging.Linear
	case "box":
		return imaging.Box
	case "mitchell":
		return imaging.MitchellNetravali
	default:
		return imaging.Lanczos
	}
}

func optionalOpts(args []object.Object, idx int) (map[string]object.Object, object.Object) {
	if len(args) <= idx {
		return nil, nil
	}
	h, ok := args[idx].(*object.Hash)
	if !ok {
		return nil, object.NewError("argument `opts` must be HASH, got %s", args[idx].Type())
	}
	out := make(map[string]object.Object, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[strings.TrimSpace(pair.Key.Inspect())] = pair.Value
	}
	return out, nil
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.String); ok {
		return s.Value, nil
	}
	return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
}

func asInt(arg object.Object, name string) (int64, object.Object) {
	switch v := arg.(type) {
	case *object.Integer:
		return v.Value, nil
	case *object.Float:
		return int64(v.Value), nil
	default:
		return 0, object.NewError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
}

func numberValue(arg object.Object) (float64, bool) {
	switch v := arg.(type) {
	case *object.Integer:
		return float64(v.Value), true
	case *object.Float:
		return v.Value, true
	default:
		return 0, false
	}
}

func optString(opts map[string]object.Object, key string) string {
	if opts == nil {
		return ""
	}
	if s, ok := opts[key].(*object.String); ok {
		return strings.TrimSpace(s.Value)
	}
	return ""
}

func normalizeImageFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "jpg":
		return "jpeg"
	case "":
		return "png"
	default:
		return format
	}
}

func mimeForImageFormat(format string) string {
	switch normalizeImageFormat(format) {
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var osReadFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
