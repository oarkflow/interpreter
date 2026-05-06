package builtins

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/oarkflow/interpreter/pkg/object"
	renderpkg "github.com/oarkflow/interpreter/pkg/render"
	"github.com/oarkflow/interpreter/pkg/security"
	_ "golang.org/x/image/webp"
)

func parseOptionalHash(arg object.Object, name string) (map[string]object.Object, object.Object) {
	if arg == nil || arg == object.NULL {
		return nil, nil
	}
	h, ok := arg.(*object.Hash)
	if !ok {
		return nil, object.NewError("argument `%s` must be HASH, got %s", name, arg.Type())
	}
	out := make(map[string]object.Object, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[strings.TrimSpace(pair.Key.Inspect())] = pair.Value
	}
	return out, nil
}

func optBool(opts map[string]object.Object, key string, def bool) bool {
	if opts == nil {
		return def
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return def
	}
	switch v := val.(type) {
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value != 0
	case *object.String:
		s := strings.ToLower(strings.TrimSpace(v.Value))
		switch s {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func optFloat64(opts map[string]object.Object, key string) (float64, bool) {
	if opts == nil {
		return 0, false
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case *object.Float:
		return v.Value, true
	case *object.Integer:
		return float64(v.Value), true
	default:
		return 0, false
	}
}

func optStringSlice(opts map[string]object.Object, key string) []string {
	if opts == nil {
		return nil
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return nil
	}
	arr, ok := val.(*object.Array)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		s, ok := el.(*object.String)
		if !ok {
			continue
		}
		part := strings.TrimSpace(s.Value)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func cloneFileValue(file *object.FileValue) *object.FileValue {
	if file == nil {
		return nil
	}
	cloned := *file
	cloned.Data = append([]byte(nil), file.Data...)
	return &cloned
}

func cloneImageValue(img *object.ImageValue) *object.ImageValue {
	if img == nil {
		return nil
	}
	cloned := *img
	cloned.Data = append([]byte(nil), img.Data...)
	return &cloned
}

func cloneTableValue(table *object.TableValue) *object.TableValue {
	if table == nil {
		return nil
	}
	cloned := *table
	cloned.Columns = append([]string(nil), table.Columns...)
	cloned.Rows = make([]map[string]object.Object, 0, len(table.Rows))
	for _, row := range table.Rows {
		cloned.Rows = append(cloned.Rows, cloneRow(row))
	}
	return &cloned
}

func cloneRow(row map[string]object.Object) map[string]object.Object {
	if row == nil {
		return nil
	}
	cloned := make(map[string]object.Object, len(row))
	for k, v := range row {
		cloned[k] = v
	}
	return cloned
}

func resolveFileInput(env *object.Environment, input object.Object, defaultKind string, opts map[string]object.Object) (*object.FileValue, object.Object) {
	switch v := input.(type) {
	case *object.FileValue:
		file := cloneFileValue(v)
		applyFileOpts(file, opts)
		return file, nil
	case *object.ImageValue:
		file, err := imageValueToFileValue(v)
		if err != nil {
			return nil, object.NewError("image conversion failed: %v", err)
		}
		applyFileOpts(file, opts)
		return file, nil
	case *object.String:
		art := &object.RenderArtifact{
			Kind:      defaultKind,
			Source:    v.Value,
			SourceTyp: detectRenderSourceType(v.Value),
			MIME:      optString(opts, "mime"),
			Name:      optString(opts, "name"),
		}
		if maxBytes, ok := optInt(opts, "max_bytes"); ok {
			art.MaxBytes = maxBytes
		}
		return loadFileFromArtifact(env, art, opts)
	case *object.RenderArtifact:
		cloned := *v
		if cloned.Kind == "" {
			cloned.Kind = defaultKind
		}
		applyRenderOpts(&cloned, opts)
		return loadFileFromArtifact(env, &cloned, opts)
	default:
		return nil, object.NewError("unsupported file source: %s", input.Type())
	}
}

func loadFileFromArtifact(env *object.Environment, art *object.RenderArtifact, opts map[string]object.Object) (*object.FileValue, object.Object) {
	res, err := renderpkg.Resolve(nil, env, art)
	if err != nil && res == nil {
		return nil, object.NewError("file load failed: %v", err)
	}
	if res == nil {
		return nil, object.NewError("file load failed")
	}
	if res.Error != "" {
		return nil, object.NewError("%s", res.Error)
	}
	data := artifactResolvedBytes(res)
	file := &object.FileValue{
		Name:       firstNonEmpty(optString(opts, "name"), res.Name),
		Path:       artifactPath(art),
		MIME:       firstNonEmpty(optString(opts, "mime"), res.MIME),
		Encoding:   firstNonEmpty(optString(opts, "encoding"), "binary"),
		SourceType: firstNonEmpty(strings.TrimSpace(art.SourceTyp), res.SourceType),
		Size:       int64(len(data)),
		Data:       data,
	}
	applyFileOpts(file, opts)
	return file, nil
}

func artifactResolvedBytes(res *renderpkg.ResolvedArtifact) []byte {
	if res == nil {
		return nil
	}
	if len(res.Bytes) > 0 {
		return append([]byte(nil), res.Bytes...)
	}
	if res.DataURL != "" {
		_, payload, ok := strings.Cut(res.DataURL, ",")
		if ok {
			if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
				return data
			}
		}
	}
	if res.Content != "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(res.MIME)), "text/") {
		return []byte(res.Content)
	}
	return []byte(res.Content)
}

func artifactPath(art *object.RenderArtifact) string {
	if art == nil {
		return ""
	}
	if strings.TrimSpace(art.SourceTyp) == "path" || detectRenderSourceType(art.Source) == "path" {
		return art.Source
	}
	return ""
}

func applyFileOpts(file *object.FileValue, opts map[string]object.Object) {
	if file == nil {
		return
	}
	if v := optString(opts, "name"); v != "" {
		file.Name = v
	}
	if v := optString(opts, "mime"); v != "" {
		file.MIME = v
	}
	if v := optString(opts, "encoding"); v != "" {
		file.Encoding = v
	}
	if file.Size == 0 && len(file.Data) > 0 {
		file.Size = int64(len(file.Data))
	}
}

func resolveImageInput(env *object.Environment, input object.Object, opts map[string]object.Object) (*object.ImageValue, object.Object) {
	switch v := input.(type) {
	case *object.ImageValue:
		img := cloneImageValue(v)
		applyImageOpts(img, opts)
		return img, nil
	default:
		file, errObj := resolveFileInput(env, input, "image", opts)
		if errObj != nil {
			return nil, errObj
		}
		return decodeImageValue(file, opts)
	}
}

func decodeImageValue(file *object.FileValue, opts map[string]object.Object) (*object.ImageValue, object.Object) {
	if file == nil {
		return nil, object.NewError("image source is null")
	}
	img, format, err := image.Decode(bytes.NewReader(file.Data))
	if err != nil {
		return nil, object.NewError("image decode failed: %v", err)
	}
	bounds := img.Bounds()
	mimeType := firstNonEmpty(file.MIME, mime.TypeByExtension("."+strings.ToLower(format)))
	if mimeType == "" {
		mimeType = http.DetectContentType(file.Data)
	}
	out := &object.ImageValue{
		Name:       file.Name,
		Path:       file.Path,
		MIME:       strings.Split(mimeType, ";")[0],
		Format:     normalizeImageFormat(format),
		SourceType: file.SourceType,
		Width:      int64(bounds.Dx()),
		Height:     int64(bounds.Dy()),
		Data:       append([]byte(nil), file.Data...),
		Image:      img,
	}
	applyImageOpts(out, opts)
	return out, nil
}

func applyImageOpts(img *object.ImageValue, opts map[string]object.Object) {
	if img == nil {
		return
	}
	if v := optString(opts, "name"); v != "" {
		img.Name = v
	}
	if v := optString(opts, "mime"); v != "" {
		img.MIME = v
	}
}

func imageValueToFileValue(img *object.ImageValue) (*object.FileValue, error) {
	data, _, mimeType, err := encodeImageValue(img, "")
	if err != nil {
		return nil, err
	}
	return &object.FileValue{
		Name:       img.Name,
		Path:       img.Path,
		MIME:       mimeType,
		Encoding:   "binary",
		SourceType: img.SourceType,
		Size:       int64(len(data)),
		Data:       data,
	}, nil
}

func encodeImageValue(img *object.ImageValue, requestedFormat string) ([]byte, string, string, error) {
	if img == nil {
		return nil, "", "", fmt.Errorf("image is null")
	}
	if requestedFormat == "" {
		requestedFormat = img.Format
	}
	format := normalizeImageFormat(requestedFormat)
	if format == "" {
		format = "png"
	}
	if img.Image == nil {
		if len(img.Data) > 0 {
			decoded, _, err := image.Decode(bytes.NewReader(img.Data))
			if err != nil {
				return nil, "", "", err
			}
			img.Image = decoded
		} else {
			return nil, "", "", fmt.Errorf("image data unavailable")
		}
	}
	var buf bytes.Buffer
	var mimeType string
	switch format {
	case "jpeg", "jpg":
		mimeType = "image/jpeg"
		if err := jpeg.Encode(&buf, img.Image, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", "", err
		}
	case "gif":
		mimeType = "image/gif"
		if err := gif.Encode(&buf, img.Image, nil); err != nil {
			return nil, "", "", err
		}
	default:
		format = "png"
		mimeType = "image/png"
		if err := png.Encode(&buf, img.Image); err != nil {
			return nil, "", "", err
		}
	}
	return buf.Bytes(), format, mimeType, nil
}

func normalizeImageFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpg":
		return "jpeg"
	case "jpeg", "png", "gif":
		return strings.ToLower(strings.TrimSpace(format))
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

func saveBytesToPath(path string, data []byte) object.Object {
	safePath, err := SanitizePathLocal(path)
	if err != nil {
		return object.NewError("%s", err)
	}
	if err := security.CheckFileWriteAllowed(safePath); err != nil {
		return object.NewError("%s", err)
	}
	if err := os.MkdirAll(filepath.Dir(safePath), 0o755); err != nil {
		return object.NewError("%s", err)
	}
	if err := os.WriteFile(safePath, data, 0o644); err != nil {
		return object.NewError("%s", err)
	}
	return &object.String{Value: safePath}
}

func loadTextFile(path string) ([]byte, string, object.Object) {
	safePath, err := SanitizePathLocal(path)
	if err != nil {
		return nil, "", object.NewError("%s", err)
	}
	if err := security.CheckFileReadAllowed(safePath); err != nil {
		return nil, "", object.NewError("%s", err)
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, "", object.NewError("%s", err)
	}
	return data, safePath, nil
}

func delimiterFromOpts(opts map[string]object.Object) rune {
	delimiter := ','
	if v := optString(opts, "delimiter"); v != "" {
		runes := []rune(v)
		if len(runes) > 0 {
			delimiter = runes[0]
		}
	}
	return delimiter
}

func decodeCSVText(text string, opts map[string]object.Object) (*object.TableValue, object.Object) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = delimiterFromOpts(opts)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, object.NewError("csv decode failed: %v", err)
	}
	if len(records) == 0 {
		return &object.TableValue{MIME: "text/csv"}, nil
	}
	headers := optStringSlice(opts, "columns")
	hasHeaders := optBool(opts, "headers", true)
	startRow := 0
	if len(headers) == 0 {
		if hasHeaders {
			headers = append([]string(nil), records[0]...)
			startRow = 1
		} else {
			headers = make([]string, len(records[0]))
			for i := range headers {
				headers[i] = fmt.Sprintf("column_%d", i+1)
			}
		}
	}
	rows := make([]map[string]object.Object, 0, maxInt(len(records)-startRow, 0))
	for _, record := range records[startRow:] {
		row := make(map[string]object.Object, len(headers))
		for i, col := range headers {
			value := ""
			if i < len(record) {
				value = record[i]
			}
			row[col] = &object.String{Value: value}
		}
		rows = append(rows, row)
	}
	return &object.TableValue{
		MIME:    "text/csv",
		Columns: headers,
		Rows:    rows,
	}, nil
}

func encodeTableCSV(value object.Object, opts map[string]object.Object) (string, *object.TableValue, object.Object) {
	table, errObj := tableFromObject(value, opts)
	if errObj != nil {
		return "", nil, errObj
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Comma = delimiterFromOpts(opts)
	if len(table.Columns) > 0 {
		if err := writer.Write(table.Columns); err != nil {
			return "", nil, object.NewError("csv encode failed: %v", err)
		}
	}
	for _, row := range table.Rows {
		record := make([]string, len(table.Columns))
		for i, col := range table.Columns {
			record[i] = rowStringValue(row[col])
		}
		if err := writer.Write(record); err != nil {
			return "", nil, object.NewError("csv encode failed: %v", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", nil, object.NewError("csv encode failed: %v", err)
	}
	return buf.String(), table, nil
}

func tableFromObject(value object.Object, opts map[string]object.Object) (*object.TableValue, object.Object) {
	switch v := value.(type) {
	case *object.TableValue:
		table := cloneTableValue(v)
		if cols := optStringSlice(opts, "columns"); len(cols) > 0 {
			table.Columns = cols
		}
		return table, nil
	case *object.Array:
		rows := make([]map[string]object.Object, 0, len(v.Elements))
		columns := optStringSlice(opts, "columns")
		seen := make(map[string]struct{})
		for _, el := range v.Elements {
			hash, ok := el.(*object.Hash)
			if !ok {
				return nil, object.NewError("table rows must be HASH, got %s", el.Type())
			}
			row := hashToRow(hash)
			rows = append(rows, row)
			if len(columns) == 0 {
				for key := range row {
					if _, exists := seen[key]; exists {
						continue
					}
					seen[key] = struct{}{}
					columns = append(columns, key)
				}
			}
		}
		if len(optStringSlice(opts, "columns")) == 0 {
			slices.Sort(columns)
		}
		return &object.TableValue{MIME: "text/csv", Columns: columns, Rows: rows}, nil
	default:
		return nil, object.NewError("expected TABLE_VALUE or ARRAY of rows, got %s", value.Type())
	}
}

func hashToRow(hash *object.Hash) map[string]object.Object {
	row := make(map[string]object.Object, len(hash.Pairs))
	for _, pair := range hash.Pairs {
		row[pair.Key.Inspect()] = pair.Value
	}
	return row
}

func rowToHash(row map[string]object.Object) *object.Hash {
	pairs := make(map[object.HashKey]object.HashPair, len(row))
	for k, v := range row {
		key := &object.String{Value: k}
		pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: pairs}
}

func rowStringValue(value object.Object) string {
	if value == nil || value == object.NULL {
		return ""
	}
	if s, ok := value.(*object.String); ok {
		return s.Value
	}
	return value.Inspect()
}

func jsonMarshalValue(value object.Object, opts map[string]object.Object) ([]byte, object.Object) {
	raw := objectToNative(value)
	if optBool(opts, "pretty", false) {
		indent := optString(opts, "indent")
		if indent == "" {
			indent = "  "
		}
		out, err := json.MarshalIndent(raw, "", indent)
		if err != nil {
			return nil, object.NewError("json encode failed: %v", err)
		}
		return out, nil
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return nil, object.NewError("json encode failed: %v", err)
	}
	return out, nil
}

func convertToRenderArtifact(value object.Object, opts map[string]object.Object) (*object.RenderArtifact, object.Object) {
	art, ok := renderpkg.ArtifactFromObject(value)
	if !ok {
		return nil, object.NewError("cannot render %s", value.Type())
	}
	cloned := *art
	applyRenderOpts(&cloned, opts)
	return &cloned, nil
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

func setImageMetadataFromImageValue(img *object.ImageValue, data []byte, format, mimeType string, src image.Image) {
	if img == nil {
		return
	}
	if src != nil {
		img.Image = src
		bounds := src.Bounds()
		img.Width = int64(bounds.Dx())
		img.Height = int64(bounds.Dy())
	}
	img.Data = append([]byte(nil), data...)
	img.Format = normalizeImageFormat(format)
	img.MIME = mimeType
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
