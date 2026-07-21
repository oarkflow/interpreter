// Package pdf registers SPL builtins for github.com/oarkflow/pdf: creating,
// merging, splitting, watermarking, encrypting, and extracting content from
// PDF files. It follows the same plugin pattern as builtins/images and
// builtins/database (own go.mod, self-registers via eval.RegisterBuiltins in
// an init(), blank-imported by hosts that want it) since the underlying
// library is a real, non-trivial dependency most embedders don't need.
package pdf

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	pdflib "github.com/oarkflow/pdf"
	"github.com/oarkflow/pdf/converter"
	"github.com/oarkflow/pdf/core"

	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"pdf_info":             {Fn: fnInfo},
		"pdf_validate":         {Fn: fnValidate},
		"pdf_merge":            {Fn: fnMerge},
		"pdf_split":            {Fn: fnSplit},
		"pdf_delete_pages":     {Fn: fnDeletePages},
		"pdf_reorder":          {Fn: fnReorder},
		"pdf_rotate":           {Fn: fnRotate},
		"pdf_compress":         {Fn: fnCompress},
		"pdf_protect":          {Fn: fnProtect},
		"pdf_decrypt":          {Fn: fnDecrypt},
		"pdf_watermark":        {Fn: fnWatermark},
		"pdf_add_page_numbers": {Fn: fnAddPageNumbers},
		"pdf_set_metadata":     {Fn: fnSetMetadata},
		"pdf_stamp_image":      {Fn: fnStampImage},
		"pdf_to_text":          {Fn: fnToText},
		"pdf_to_html":          {Fn: fnToHTML},
		"pdf_to_markdown":      {Fn: fnToMarkdown},
		"pdf_to_json":          {Fn: fnToJSON},
		"pdf_search":           {Fn: fnSearch},
		"pdf_extract_images":   {Fn: fnExtractImages},
		"pdf_from_html":        {Fn: fnFromHTML},
		"pdf_from_markdown":    {Fn: fnFromMarkdown},
		"pdf_from_url":         {Fn: fnFromURL},
		"pdf_images_to_pdf":    {Fn: fnImagesToPDF},
		"pdf_fill_form":        {Fn: fnFillForm},
		"pdf_list_form_fields": {Fn: fnListFormFields},
		"pdf_quick":            {Fn: fnQuick},
	})
}

// ---------------------------------------------------------------------------
// Argument / security helpers
// ---------------------------------------------------------------------------

func argString(args []object.Object, idx int, name string) (string, object.Object) {
	if idx >= len(args) {
		return "", object.NewError("pdf: missing argument `%s`", name)
	}
	s, ok := args[idx].(*object.String)
	if !ok {
		return "", object.NewError("pdf: argument `%s` must be STRING, got %s", name, args[idx].Type())
	}
	return s.Value, nil
}

func optString(args []object.Object, idx int, fallback string) string {
	if idx >= len(args) {
		return fallback
	}
	if s, ok := args[idx].(*object.String); ok {
		return s.Value
	}
	return fallback
}

func optHash(args []object.Object, idx int) map[string]object.Object {
	if idx >= len(args) {
		return nil
	}
	h, ok := args[idx].(*object.Hash)
	if !ok {
		return nil
	}
	out := make(map[string]object.Object, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[strings.TrimSpace(pair.Key.Inspect())] = pair.Value
	}
	return out
}

func hashString(opts map[string]object.Object, key, fallback string) string {
	if opts == nil {
		return fallback
	}
	if s, ok := opts[key].(*object.String); ok {
		return s.Value
	}
	return fallback
}

func hashFloat(opts map[string]object.Object, key string, fallback float64) float64 {
	if opts == nil {
		return fallback
	}
	switch v := opts[key].(type) {
	case *object.Float:
		return v.Value
	case *object.Integer:
		return float64(v.Value)
	default:
		return fallback
	}
}

func hashBool(opts map[string]object.Object, key string, fallback bool) bool {
	if opts == nil {
		return fallback
	}
	if b, ok := opts[key].(*object.Boolean); ok {
		return b.Value
	}
	return fallback
}

func stringArray(arg object.Object, name string) ([]string, object.Object) {
	arr, ok := arg.(*object.Array)
	if !ok {
		return nil, object.NewError("pdf: argument `%s` must be ARRAY, got %s", name, arg.Type())
	}
	out := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		s, ok := el.(*object.String)
		if !ok {
			return nil, object.NewError("pdf: argument `%s` must be an array of strings", name)
		}
		out = append(out, s.Value)
	}
	return out, nil
}

func pageSpec(spec string) ([]int, object.Object) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	pages, err := pdflib.ParsePageSpec(spec)
	if err != nil {
		return nil, object.NewError("pdf: invalid page spec %q: %v", spec, err)
	}
	return pages, nil
}

func checkRead(path string) (string, object.Object) {
	safe, err := builtinspkg.SanitizePathLocal(path)
	if err != nil {
		return "", object.NewError("pdf: %v", err)
	}
	if err := security.CheckFileReadAllowed(safe); err != nil {
		return "", object.NewError("pdf: %v", err)
	}
	return safe, nil
}

func checkWrite(path string) (string, object.Object) {
	safe, err := builtinspkg.SanitizePathLocal(path)
	if err != nil {
		return "", object.NewError("pdf: %v", err)
	}
	if err := security.CheckFileWriteAllowed(safe); err != nil {
		return "", object.NewError("pdf: %v", err)
	}
	return safe, nil
}

// structToObject converts a Go value (typically a json-tagged struct
// returned by github.com/oarkflow/pdf) into an SPL object via a JSON
// marshal/unmarshal round-trip, so hash keys follow the library's own json
// tags (snake/camel case as the library defines them) instead of raw Go
// field names, which eval.ToObject's struct-reflection fallback would use
// verbatim.
func structToObject(v any) object.Object {
	data, err := json.Marshal(v)
	if err != nil {
		return object.NewError("pdf: encode result: %v", err)
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return object.NewError("pdf: decode result: %v", err)
	}
	return jsonToObject(generic)
}

// jsonToObject converts a decoded-JSON value (map[string]any/[]any/float64/
// ...) into an SPL object.Object. Unlike eval.ToObject, whole-number
// float64 values (encoding/json has no separate int type when decoding into
// interface{}) become object.Integer rather than object.Float, so fields
// like "pages" behave as plain SPL integers.
func jsonToObject(v any) object.Object {
	switch val := v.(type) {
	case nil:
		return object.NULL
	case bool:
		return object.NativeBoolToBooleanObject(val)
	case float64:
		if !math.IsInf(val, 0) && !math.IsNaN(val) && val == math.Trunc(val) {
			return &object.Integer{Value: int64(val)}
		}
		return &object.Float{Value: val}
	case string:
		return &object.String{Value: val}
	case []any:
		elements := make([]object.Object, len(val))
		for i, e := range val {
			elements[i] = jsonToObject(e)
		}
		return &object.Array{Elements: elements}
	case map[string]any:
		pairs := make(map[object.HashKey]object.HashPair, len(val))
		for k, vv := range val {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: jsonToObject(vv)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return object.NewError("pdf: unexpected JSON value type %T", v)
	}
}

func okOrError(err error) object.Object {
	if err != nil {
		return object.NewError("pdf: %v", err)
	}
	return object.TRUE
}

func convertOptions(password string, pages []int) converter.ConvertOptions {
	return converter.ConvertOptions{Pages: pages, Password: password}
}

// ---------------------------------------------------------------------------
// Inspection
// ---------------------------------------------------------------------------

func fnInfo(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("pdf_info: wrong number of arguments. got=%d, want=1 or 2", len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	password := optString(args, 1, "")
	var info *pdflib.PDFInfo
	var err error
	if password != "" {
		info, err = pdflib.Info(safe, password)
	} else {
		info, err = pdflib.Info(safe)
	}
	if err != nil {
		return object.NewError("pdf_info: %v", err)
	}
	return structToObject(info)
}

func fnValidate(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("pdf_validate: wrong number of arguments. got=%d, want=1 or 2", len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	result := pdflib.ValidatePDF(safe, optString(args, 1, ""))
	return structToObject(result)
}

// ---------------------------------------------------------------------------
// Page-level operations
// ---------------------------------------------------------------------------

func fnMerge(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("pdf_merge: wrong number of arguments. got=%d, want=2 or more (output, inputs...)", len(args))
	}
	out, errObj := argString(args, 0, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	inputs := make([]string, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		in, errObj := argString(args, i, "inputPath")
		if errObj != nil {
			return errObj
		}
		safeIn, errObj := checkRead(in)
		if errObj != nil {
			return errObj
		}
		inputs = append(inputs, safeIn)
	}
	return okOrError(pdflib.Merge(safeOut, inputs...))
}

func fnSplit(args ...object.Object) object.Object {
	return pageOpFn(args, "pdf_split", func(in, out string, pages []int, opts converter.ConvertOptions) error {
		return pdflib.Split(in, out, pages, opts)
	})
}

func fnDeletePages(args ...object.Object) object.Object {
	return pageOpFn(args, "pdf_delete_pages", func(in, out string, pages []int, opts converter.ConvertOptions) error {
		return pdflib.DeletePages(in, out, pages, opts)
	})
}

func fnReorder(args ...object.Object) object.Object {
	return pageOpFn(args, "pdf_reorder", func(in, out string, pages []int, opts converter.ConvertOptions) error {
		return pdflib.Reorder(in, out, pages, opts)
	})
}

// pageOpFn implements the common (inputPath, outputPath, pageSpec,
// [password]) shape shared by split/delete_pages/reorder.
func pageOpFn(args []object.Object, fname string, do func(in, out string, pages []int, opts converter.ConvertOptions) error) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("%s: wrong number of arguments. got=%d, want=3 or 4", fname, len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	spec, errObj := argString(args, 2, "pageSpec")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	pages, errObj := pageSpec(spec)
	if errObj != nil {
		return errObj
	}
	password := optString(args, 3, "")
	return okOrError(do(safeIn, safeOut, pages, convertOptions(password, nil)))
}

func fnRotate(args ...object.Object) object.Object {
	if len(args) < 4 || len(args) > 5 {
		return object.NewError("pdf_rotate: wrong number of arguments. got=%d, want=4 or 5", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	spec, errObj := argString(args, 2, "pageSpec")
	if errObj != nil {
		return errObj
	}
	degrees, ok := args[3].(*object.Integer)
	if !ok {
		return object.NewError("pdf_rotate: argument `degrees` must be INTEGER, got %s", args[3].Type())
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	pages, errObj := pageSpec(spec)
	if errObj != nil {
		return errObj
	}
	password := optString(args, 4, "")
	return okOrError(pdflib.Rotate(safeIn, safeOut, pages, int(degrees.Value), convertOptions(password, nil)))
}

// ---------------------------------------------------------------------------
// Security / compression
// ---------------------------------------------------------------------------

func fnCompress(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_compress: wrong number of arguments. got=%d, want=2 or 3", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	return okOrError(pdflib.CompressPDF(safeIn, safeOut, optString(args, 2, "")))
}

// encryptionAlgorithm defaults to AES-128 (fully supported end-to-end by
// github.com/oarkflow/pdf@v0.0.2 as of this writing) rather than AES-256,
// which that library version accepts for some operations but rejects
// during Protect() with "AES-256 ... is not supported yet". Callers can
// still opt into "aes-256" explicitly if a future library version supports
// it end-to-end.
func encryptionAlgorithm(name string) core.EncryptionAlgorithm {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rc4-128", "rc4_128", "rc4":
		return core.RC4_128
	case "aes-256", "aes_256":
		return core.AES_256
	default:
		return core.AES_128
	}
}

func fnProtect(args ...object.Object) object.Object {
	if len(args) < 4 || len(args) > 5 {
		return object.NewError("pdf_protect: wrong number of arguments. got=%d, want=4 or 5 (input, output, userPassword, ownerPassword, [algorithm])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	userPassword, errObj := argString(args, 2, "userPassword")
	if errObj != nil {
		return errObj
	}
	ownerPassword, errObj := argString(args, 3, "ownerPassword")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	cfg := core.EncryptionConfig{
		Algorithm:     encryptionAlgorithm(optString(args, 4, "aes-128")),
		UserPassword:  userPassword,
		OwnerPassword: ownerPassword,
	}
	return okOrError(pdflib.Protect(safeIn, safeOut, cfg))
}

func fnDecrypt(args ...object.Object) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("pdf_decrypt: wrong number of arguments. got=%d, want=3 or 4 (input, output, password)", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	password, errObj := argString(args, 2, "password")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	return okOrError(pdflib.Decrypt(safeIn, safeOut, converter.ConvertOptions{Password: password}))
}

// ---------------------------------------------------------------------------
// Stamping / annotation
// ---------------------------------------------------------------------------

func fnWatermark(args ...object.Object) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("pdf_watermark: wrong number of arguments. got=%d, want=3 or 4 (input, output, text, [opts])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	text, errObj := argString(args, 2, "text")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 3)
	var pages []int
	if spec := hashString(opts, "pages", ""); spec != "" {
		var errObj object.Object
		pages, errObj = pageSpec(spec)
		if errObj != nil {
			return errObj
		}
	}
	wOpts := pdflib.WatermarkOptions{
		Text:     text,
		FontSize: hashFloat(opts, "font_size", 48),
		Opacity:  hashFloat(opts, "opacity", 0.3),
		Angle:    hashFloat(opts, "angle", 45),
		Pages:    pages,
		Password: hashString(opts, "password", ""),
	}
	return okOrError(pdflib.Watermark(safeIn, safeOut, wOpts))
}

func fnAddPageNumbers(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_add_page_numbers: wrong number of arguments. got=%d, want=2 or 3 (input, output, [opts])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 2)
	pOpts := pdflib.PageNumberOptions{
		Format:   hashString(opts, "format", "Page %d of %d"),
		FontSize: hashFloat(opts, "font_size", 10),
		Margin:   hashFloat(opts, "margin", 36),
		Password: hashString(opts, "password", ""),
	}
	return okOrError(pdflib.AddPageNumbers(safeIn, safeOut, pOpts))
}

func fnSetMetadata(args ...object.Object) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("pdf_set_metadata: wrong number of arguments. got=%d, want=3 or 4 (input, output, metadata, [password])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	metaHash, ok := args[2].(*object.Hash)
	if !ok {
		return object.NewError("pdf_set_metadata: argument `metadata` must be HASH, got %s", args[2].Type())
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	metadata := make(map[string]string, len(metaHash.Pairs))
	for _, pair := range metaHash.Pairs {
		if s, ok := pair.Value.(*object.String); ok {
			metadata[strings.TrimSpace(pair.Key.Inspect())] = s.Value
		}
	}
	return okOrError(pdflib.SetMetadata(safeIn, safeOut, metadata, converter.ConvertOptions{Password: optString(args, 3, "")}))
}

func fnStampImage(args ...object.Object) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("pdf_stamp_image: wrong number of arguments. got=%d, want=3 or 4 (input, output, imagePath, [opts])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	imagePath, errObj := argString(args, 2, "imagePath")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	safeImg, errObj := checkRead(imagePath)
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 3)
	var pages []int
	if spec := hashString(opts, "pages", ""); spec != "" {
		var errObj object.Object
		pages, errObj = pageSpec(spec)
		if errObj != nil {
			return errObj
		}
	}
	sOpts := pdflib.ImageStampOptions{
		ImagePath: safeImg,
		Page:      int(hashFloat(opts, "page", 1)),
		Pages:     pages,
		X:         hashFloat(opts, "x", 36),
		Y:         hashFloat(opts, "y", 36),
		Width:     hashFloat(opts, "width", 100),
		Height:    hashFloat(opts, "height", 50),
		Password:  hashString(opts, "password", ""),
	}
	return okOrError(pdflib.StampImage(safeIn, safeOut, sOpts))
}

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

func fnToText(args ...object.Object) object.Object {
	return extractFn(args, "pdf_to_text", pdflib.ToText)
}

func fnToHTML(args ...object.Object) object.Object {
	return extractFn(args, "pdf_to_html", pdflib.ToHTML)
}

func fnToMarkdown(args ...object.Object) object.Object {
	return extractFn(args, "pdf_to_markdown", pdflib.ToMarkdown)
}

func extractFn(args []object.Object, fname string, do func(path string, opts ...converter.ConvertOptions) (string, error)) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("%s: wrong number of arguments. got=%d, want=1 or 2", fname, len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	result, err := do(safe, converter.ConvertOptions{Password: optString(args, 1, "")})
	if err != nil {
		return object.NewError("%s: %v", fname, err)
	}
	return &object.String{Value: result}
}

func fnToJSON(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("pdf_to_json: wrong number of arguments. got=%d, want=1 or 2", len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	data, err := pdflib.ToJSON(safe, converter.ConvertOptions{Password: optString(args, 1, "")})
	if err != nil {
		return object.NewError("pdf_to_json: %v", err)
	}
	return &object.String{Value: string(data)}
}

func fnSearch(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_search: wrong number of arguments. got=%d, want=2 or 3 (input, query, [opts])", len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	query, errObj := argString(args, 1, "query")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 2)
	matches, err := pdflib.SearchText(safe, pdflib.SearchOptions{
		Query:         query,
		CaseSensitive: hashBool(opts, "case_sensitive", false),
		Password:      hashString(opts, "password", ""),
	})
	if err != nil {
		return object.NewError("pdf_search: %v", err)
	}
	return structToObject(matches)
}

func fnExtractImages(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_extract_images: wrong number of arguments. got=%d, want=2 or 3 (input, outputDir, [password])", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	outDir, errObj := argString(args, 1, "outputDir")
	if errObj != nil {
		return errObj
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOutDir, errObj := checkWrite(outDir)
	if errObj != nil {
		return errObj
	}
	images, err := pdflib.ExtractImages(safeIn, converter.ConvertOptions{Password: optString(args, 2, ""), ExtractImages: true})
	if err != nil {
		return object.NewError("pdf_extract_images: %v", err)
	}
	if err := ensureDir(safeOutDir); err != nil {
		return object.NewError("pdf_extract_images: %v", err)
	}
	paths := make([]object.Object, 0, len(images))
	for i, img := range images {
		ext := ".png"
		if strings.Contains(img.MimeType, "jpeg") {
			ext = ".jpg"
		}
		name := joinPath(safeOutDir, indexedName("image", i+1, ext))
		if err := writeFile(name, img.Data); err != nil {
			return object.NewError("pdf_extract_images: %v", err)
		}
		paths = append(paths, &object.String{Value: name})
	}
	return &object.Array{Elements: paths}
}

// ---------------------------------------------------------------------------
// Generation
// ---------------------------------------------------------------------------

func fnFromHTML(args ...object.Object) object.Object {
	if len(args) != 2 {
		return object.NewError("pdf_from_html: wrong number of arguments. got=%d, want=2", len(args))
	}
	htmlContent, errObj := argString(args, 0, "html")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	return okOrError(pdflib.FromHTML(htmlContent, safeOut))
}

func fnFromMarkdown(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_from_markdown: wrong number of arguments. got=%d, want=2 or 3 (markdown, output, [opts])", len(args))
	}
	markdown, errObj := argString(args, 0, "markdown")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 2)
	mOpts := pdflib.MarkdownOptions{
		Title:    hashString(opts, "title", ""),
		Author:   hashString(opts, "author", ""),
		Theme:    hashString(opts, "theme", "classic"),
		Margin:   hashFloat(opts, "margin", 54),
		PageSize: hashString(opts, "page_size", "a4"),
		TOC:      hashBool(opts, "toc", false),
	}
	return okOrError(pdflib.FromMarkdown(markdown, safeOut, mOpts))
}

func fnFromURL(args ...object.Object) object.Object {
	if len(args) != 2 {
		return object.NewError("pdf_from_url: wrong number of arguments. got=%d, want=2", len(args))
	}
	if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
		return object.NewError("pdf_from_url: %v", err)
	}
	url, errObj := argString(args, 0, "url")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	return okOrError(pdflib.FromURL(url, safeOut))
}

func fnImagesToPDF(args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("pdf_images_to_pdf: wrong number of arguments. got=%d, want=2 or 3 (output, imagePaths, [opts])", len(args))
	}
	out, errObj := argString(args, 0, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	imagePaths, errObj := stringArray(args[1], "imagePaths")
	if errObj != nil {
		return errObj
	}
	inputs := make([]pdflib.MergeInput, 0, len(imagePaths))
	for _, p := range imagePaths {
		safeIn, errObj := checkRead(p)
		if errObj != nil {
			return errObj
		}
		inputs = append(inputs, pdflib.MergeInput{Path: safeIn})
	}
	opts := optHash(args, 2)
	pageOpts := pdflib.MergePageOptions{
		Size:     hashString(opts, "size", "a4"),
		Margin:   hashFloat(opts, "margin", 0),
		ImageFit: hashString(opts, "image_fit", "contain"),
	}
	return okOrError(pdflib.ImagesToPDF(pdflib.ImagePDFOptions{Output: safeOut, Page: pageOpts, Inputs: inputs}))
}

func fnFillForm(args ...object.Object) object.Object {
	if len(args) != 3 {
		return object.NewError("pdf_fill_form: wrong number of arguments. got=%d, want=3 (input, output, data)", len(args))
	}
	in, errObj := argString(args, 0, "inputPath")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	dataHash, ok := args[2].(*object.Hash)
	if !ok {
		return object.NewError("pdf_fill_form: argument `data` must be HASH, got %s", args[2].Type())
	}
	safeIn, errObj := checkRead(in)
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	data := make(map[string]string, len(dataHash.Pairs))
	for _, pair := range dataHash.Pairs {
		data[strings.TrimSpace(pair.Key.Inspect())] = pair.Value.Inspect()
	}
	return okOrError(pdflib.FillFormFile(safeIn, safeOut, data))
}

func fnListFormFields(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("pdf_list_form_fields: wrong number of arguments. got=%d, want=1 or 2", len(args))
	}
	path, errObj := argString(args, 0, "path")
	if errObj != nil {
		return errObj
	}
	safe, errObj := checkRead(path)
	if errObj != nil {
		return errObj
	}
	fields, err := pdflib.ListFormFields(safe, optString(args, 1, ""))
	if err != nil {
		return object.NewError("pdf_list_form_fields: %v", err)
	}
	return structToObject(fields)
}

func fnQuick(args ...object.Object) object.Object {
	if len(args) != 2 {
		return object.NewError("pdf_quick: wrong number of arguments. got=%d, want=2 (text, outputPath)", len(args))
	}
	text, errObj := argString(args, 0, "text")
	if errObj != nil {
		return errObj
	}
	out, errObj := argString(args, 1, "outputPath")
	if errObj != nil {
		return errObj
	}
	safeOut, errObj := checkWrite(out)
	if errObj != nil {
		return errObj
	}
	return okOrError(pdflib.Quick(text, safeOut))
}

// ---------------------------------------------------------------------------
// Small filesystem helpers for pdf_extract_images
// ---------------------------------------------------------------------------

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func joinPath(dir, name string) string {
	return filepath.Join(dir, name)
}

func indexedName(prefix string, index int, ext string) string {
	return fmt.Sprintf("%s-%03d%s", prefix, index, ext)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
