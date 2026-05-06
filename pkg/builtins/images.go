package builtins

import (
	"bytes"
	"image"
	"image/color"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"image_load": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				img, errObj := resolveImageInput(env, args[0], opts)
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
				opts := map[string]object.Object(nil)
				if len(args) == 4 {
					opts, errObj = parseOptionalHash(args[3], "opts")
					if errObj != nil {
						return errObj
					}
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				resized := imaging.Resize(src, int(width), int(height), imageFilterFromOpts(opts))
				derived, errObj := buildDerivedImageValue(img, resized, optString(opts, "format"))
				if errObj != nil {
					return errObj
				}
				return derived
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
				bounds := src.Bounds()
				rect := image.Rect(int(x), int(y), int(x+width), int(y+height)).Intersect(bounds)
				if rect.Empty() {
					return object.NewError("crop rectangle is outside image bounds")
				}
				cropped := imaging.Crop(src, rect)
				derived, errObj := buildDerivedImageValue(img, cropped, "")
				if errObj != nil {
					return errObj
				}
				return derived
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
				degrees, ok := optFloat64(map[string]object.Object{"degrees": args[1]}, "degrees")
				if !ok {
					return object.NewError("argument `degrees` must be numeric")
				}
				opts := map[string]object.Object(nil)
				if len(args) == 3 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[2], "opts")
					if errObj != nil {
						return errObj
					}
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				bg := color.NRGBA{0, 0, 0, 0}
				rotated := imaging.Rotate(src, degrees, bg)
				derived, errObj := buildDerivedImageValue(img, rotated, optString(opts, "format"))
				if errObj != nil {
					return errObj
				}
				return derived
			},
		},
		"image_convert": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				format, errObj := asString(args[1], "format")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 3 {
					opts, errObj = parseOptionalHash(args[2], "opts")
					if errObj != nil {
						return errObj
					}
				}
				converted := cloneImageValue(img)
				data, normalized, mimeType, err := encodeImageValue(converted, format)
				if err != nil {
					return object.NewError("image convert failed: %v", err)
				}
				setImageMetadataFromImageValue(converted, data, normalized, mimeType, converted.Image)
				applyImageOpts(converted, opts)
				return converted
			},
		},
		"image_save": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("wrong number of arguments. got=%d, want=2 or 3", len(args))
				}
				img, ok := args[0].(*object.ImageValue)
				if !ok {
					return object.NewError("argument `image` must be IMAGE_VALUE, got %s", args[0].Type())
				}
				path, errObj := asString(args[1], "path")
				if errObj != nil {
					return errObj
				}
				opts := map[string]object.Object(nil)
				if len(args) == 3 {
					opts, errObj = parseOptionalHash(args[2], "opts")
					if errObj != nil {
						return errObj
					}
				}
				format := optString(opts, "format")
				if format == "" {
					ext := filepath.Ext(path)
					if ext != "" {
						format = ext[1:]
					}
				}
				data, normalized, mimeType, err := encodeImageValue(img, format)
				if err != nil {
					return object.NewError("image save failed: %v", err)
				}
				if result := saveBytesToPath(path, data); object.IsError(result) {
					return result
				}
				saved := cloneImageValue(img)
				safePath, _ := SanitizePathLocal(path)
				saved.Path = safePath
				saved.Name = firstNonEmpty(optString(opts, "name"), filepath.Base(safePath), saved.Name)
				setImageMetadataFromImageValue(saved, data, normalized, mimeType, saved.Image)
				applyImageOpts(saved, opts)
				return saved
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
				return eval.ToObject(map[string]interface{}{
					"name":        img.Name,
					"path":        img.Path,
					"mime":        img.MIME,
					"format":      img.Format,
					"source_type": img.SourceType,
					"width":       img.Width,
					"height":      img.Height,
					"size":        len(img.Data),
				})
			},
		},
		"image_render": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				art, errObj := convertToRenderArtifact(args[0], opts)
				if errObj != nil {
					return errObj
				}
				return art
			},
		},
		"image_resize_file": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) < 4 || len(args) > 5 {
					return object.NewError("wrong number of arguments. got=%d, want=4 or 5", len(args))
				}
				opts := map[string]object.Object(nil)
				if len(args) == 5 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[4], "opts")
					if errObj != nil {
						return errObj
					}
				}
				img, errObj := resolveImageInput(env, args[0], opts)
				if errObj != nil {
					return errObj
				}
				width, errObj := asInt(args[2], "width")
				if errObj != nil {
					return errObj
				}
				height, errObj := asInt(args[3], "height")
				if errObj != nil {
					return errObj
				}
				src, errObj := ensureDecodedImage(img)
				if errObj != nil {
					return errObj
				}
				resized := imaging.Resize(src, int(width), int(height), imageFilterFromOpts(opts))
				derived, errObj := buildDerivedImageValue(img, resized, optString(opts, "format"))
				if errObj != nil {
					return errObj
				}
				dstPath, errObj := asString(args[1], "dst")
				if errObj != nil {
					return errObj
				}
				return eval.Builtins["image_save"].Fn(derived, &object.String{Value: dstPath})
			},
		},
		"image_convert_file": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) < 3 || len(args) > 4 {
					return object.NewError("wrong number of arguments. got=%d, want=3 or 4", len(args))
				}
				opts := map[string]object.Object(nil)
				if len(args) == 4 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[3], "opts")
					if errObj != nil {
						return errObj
					}
				}
				img, errObj := resolveImageInput(env, args[0], opts)
				if errObj != nil {
					return errObj
				}
				format, errObj := asString(args[2], "format")
				if errObj != nil {
					return errObj
				}
				converted := eval.Builtins["image_convert"].Fn(img, &object.String{Value: format})
				if object.IsError(converted) {
					return converted
				}
				dstPath, errObj := asString(args[1], "dst")
				if errObj != nil {
					return errObj
				}
				return eval.Builtins["image_save"].Fn(converted, &object.String{Value: dstPath})
			},
		},
	})
}

func ensureDecodedImage(img *object.ImageValue) (image.Image, object.Object) {
	if img == nil {
		return nil, object.NewError("image is null")
	}
	if img.Image != nil {
		return img.Image, nil
	}
	if len(img.Data) == 0 {
		return nil, object.NewError("image data unavailable")
	}
	decoded, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return nil, object.NewError("image decode failed: %v", err)
	}
	img.Image = decoded
	return decoded, nil
}

func buildDerivedImageValue(base *object.ImageValue, derived image.Image, format string) (*object.ImageValue, object.Object) {
	next := cloneImageValue(base)
	next.Image = derived
	data, normalized, mimeType, err := encodeImageValue(next, format)
	if err != nil {
		return nil, object.NewError("image transform failed: %v", err)
	}
	setImageMetadataFromImageValue(next, data, normalized, mimeType, derived)
	next.Path = ""
	next.SourceType = "data"
	return next, nil
}
