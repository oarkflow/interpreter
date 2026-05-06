package render

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"html"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oarkflow/interpreter/pkg/config"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

type ResolvedArtifact struct {
	Kind       string `json:"kind"`
	SourceType string `json:"source_type"`
	Source     string `json:"source,omitempty"`
	Name       string `json:"name,omitempty"`
	MIME       string `json:"mime,omitempty"`
	Alt        string `json:"alt,omitempty"`
	Width      int64  `json:"width,omitempty"`
	Height     int64  `json:"height,omitempty"`
	Size       int64  `json:"size,omitempty"`
	Content    string `json:"content,omitempty"`
	DataURL    string `json:"data_url,omitempty"`
	Error      string `json:"error,omitempty"`
	Bytes      []byte `json:"-"`
}

func ConfigForEnv(env *object.Environment) *object.RenderConfig {
	if env != nil && env.RenderConfig != nil {
		return env.RenderConfig
	}
	return object.DefaultRenderConfig()
}

func Metadata(art *object.RenderArtifact) string {
	if art == nil {
		return "<render artifact>"
	}
	return art.Inspect()
}

func Resolve(ctx context.Context, env *object.Environment, art *object.RenderArtifact) (*ResolvedArtifact, error) {
	if art == nil {
		return nil, fmt.Errorf("render artifact is null")
	}
	cfg := ConfigForEnv(env)
	maxBytes := cfg.MaxBytes
	if art.MaxBytes > 0 {
		maxBytes = art.MaxBytes
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	res := &ResolvedArtifact{
		Kind:       normalizedKind(art),
		SourceType: sourceType(art),
		Source:     art.Source,
		Name:       art.Name,
		MIME:       strings.TrimSpace(art.MIME),
		Alt:        art.Alt,
		Width:      art.Width,
		Height:     art.Height,
	}

	var data []byte
	var err error
	switch res.SourceType {
	case "data":
		data, res.MIME, err = resolveData(art.Source, res.MIME, maxBytes)
	case "url":
		data, res.MIME, err = resolveURL(ctx, env, cfg, art.Source, res.MIME, maxBytes)
	default:
		data, res.MIME, res.Name, err = resolvePath(art.Source, res.MIME, res.Name, maxBytes)
	}
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	res.Size = int64(len(data))
	res.Bytes = append([]byte(nil), data...)
	if res.Name == "" {
		res.Name = defaultName(art.Source, res.SourceType)
	}
	if res.MIME == "" {
		res.MIME = http.DetectContentType(data)
	}
	if shouldExposeText(res.MIME) {
		res.Content = string(data)
	}
	if strings.HasPrefix(strings.ToLower(res.MIME), "image/") {
		res.DataURL = "data:" + res.MIME + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	return res, nil
}

func RenderObjectForTerminal(env *object.Environment, obj object.Object) (string, error) {
	art, ok := obj.(*object.RenderArtifact)
	if !ok {
		if obj == nil {
			return "null", nil
		}
		return obj.Inspect(), nil
	}
	cfg := ConfigForEnv(env)
	mode := strings.ToLower(strings.TrimSpace(art.Mode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	}
	if mode == "off" || mode == "metadata" {
		return Metadata(art), nil
	}
	res, err := Resolve(context.Background(), env, art)
	if err != nil {
		return "", err
	}
	return RenderResolvedForTerminal(res, cfg, mode), nil
}

func RenderResolvedForTerminal(res *ResolvedArtifact, cfg *object.RenderConfig, mode string) string {
	if res == nil {
		return "<render artifact>"
	}
	if res.Error != "" {
		return fmt.Sprintf("<%s render error: %s>", res.Kind, res.Error)
	}
	lowerMime := strings.ToLower(res.MIME)
	if shouldExposeText(lowerMime) && strings.TrimSpace(res.Content) != "" {
		if strings.Contains(lowerMime, "html") {
			return stripHTML(res.Content)
		}
		return res.Content
	}
	if strings.HasPrefix(lowerMime, "image/") && res.DataURL != "" {
		protocol := terminalProtocol(cfg)
		switch protocol {
		case "kitty":
			return kittyImage(res)
		case "iterm":
			return itermImage(res)
		case "sixel":
			return fmt.Sprintf("<image %s %s; sixel rendering requires pre-encoded sixel data>", res.Name, res.MIME)
		}
	}
	return fmt.Sprintf("<%s %s %s %d bytes>", res.Kind, res.Name, res.MIME, res.Size)
}

func normalizedKind(art *object.RenderArtifact) string {
	kind := strings.ToLower(strings.TrimSpace(art.Kind))
	if kind == "" {
		kind = "file"
	}
	if kind == "image" || kind == "html" || kind == "text" {
		return kind
	}
	return "file"
}

func sourceType(art *object.RenderArtifact) string {
	if art == nil {
		return "path"
	}
	if typ := strings.ToLower(strings.TrimSpace(art.SourceTyp)); typ != "" {
		return typ
	}
	src := strings.TrimSpace(art.Source)
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		return "data"
	}
	u, err := url.Parse(src)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return "url"
	}
	return "path"
}

func resolvePath(path, explicitMIME, name string, maxBytes int64) ([]byte, string, string, error) {
	safePath, err := config.SanitizePathFn(path)
	if err != nil {
		return nil, explicitMIME, name, err
	}
	if err := security.CheckFileReadAllowed(safePath); err != nil {
		return nil, explicitMIME, name, err
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return nil, explicitMIME, name, err
	}
	if info.IsDir() {
		return nil, explicitMIME, name, fmt.Errorf("cannot render directory %q", path)
	}
	if info.Size() > maxBytes {
		return nil, explicitMIME, name, fmt.Errorf("artifact exceeds %d bytes", maxBytes)
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, explicitMIME, name, err
	}
	mimeType := detectMIME(explicitMIME, safePath, data)
	if name == "" {
		name = filepath.Base(safePath)
	}
	return data, mimeType, name, nil
}

func resolveData(source, explicitMIME string, maxBytes int64) ([]byte, string, error) {
	raw := strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		header, payload, ok := strings.Cut(raw, ",")
		if !ok {
			return nil, explicitMIME, fmt.Errorf("invalid data URI")
		}
		mimeType := explicitMIME
		meta := strings.TrimPrefix(header, "data:")
		parts := strings.Split(meta, ";")
		if mimeType == "" && len(parts) > 0 && strings.Contains(parts[0], "/") {
			mimeType = parts[0]
		}
		var data []byte
		var err error
		if strings.Contains(strings.ToLower(meta), ";base64") {
			data, err = base64.StdEncoding.DecodeString(payload)
		} else {
			decoded, derr := url.QueryUnescape(payload)
			if derr != nil {
				return nil, mimeType, derr
			}
			data = []byte(decoded)
		}
		if err != nil {
			return nil, mimeType, err
		}
		if int64(len(data)) > maxBytes {
			return nil, mimeType, fmt.Errorf("artifact exceeds %d bytes", maxBytes)
		}
		return data, detectMIME(mimeType, "", data), nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		data = []byte(raw)
	}
	if int64(len(data)) > maxBytes {
		return nil, explicitMIME, fmt.Errorf("artifact exceeds %d bytes", maxBytes)
	}
	return data, detectMIME(explicitMIME, "", data), nil
}

func resolveURL(ctx context.Context, env *object.Environment, cfg *object.RenderConfig, target, explicitMIME string, maxBytes int64) ([]byte, string, error) {
	if cfg == nil || !cfg.AllowURLs {
		return nil, explicitMIME, fmt.Errorf("URL rendering is disabled")
	}
	if err := security.CheckNetworkAllowed(target); err != nil {
		return nil, explicitMIME, err
	}
	if len(cfg.AllowURLHosts) > 0 {
		host, err := security.HostFromTarget(target)
		if err != nil {
			return nil, explicitMIME, err
		}
		ok := false
		for _, allow := range cfg.AllowURLHosts {
			if security.MatchHostPattern(host, allow) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, explicitMIME, fmt.Errorf("render URL host not allowed: %s", host)
		}
	}
	timeout := 5 * time.Second
	if env != nil && env.RuntimeLimits != nil && !env.RuntimeLimits.Deadline.IsZero() {
		if until := time.Until(env.RuntimeLimits.Deadline); until > 0 && until < timeout {
			timeout = until
		}
	}
	client := &http.Client{Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return security.CheckNetworkAllowed(req.URL.String())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, explicitMIME, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, explicitMIME, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, explicitMIME, fmt.Errorf("URL render returned HTTP %d", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, explicitMIME, err
	}
	if int64(len(data)) > maxBytes {
		return nil, explicitMIME, fmt.Errorf("artifact exceeds %d bytes", maxBytes)
	}
	mimeType := explicitMIME
	if mimeType == "" {
		mimeType = strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	}
	return data, detectMIME(mimeType, resp.Request.URL.Path, data), nil
}

func detectMIME(explicit, path string, data []byte) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	if path != "" {
		if byExt := mime.TypeByExtension(filepath.Ext(path)); byExt != "" {
			return strings.Split(byExt, ";")[0]
		}
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

func shouldExposeText(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") ||
		strings.Contains(mimeType, "javascript") ||
		strings.Contains(mimeType, "markdown")
}

func defaultName(source, typ string) string {
	if typ == "url" {
		if u, err := url.Parse(source); err == nil {
			return filepath.Base(u.Path)
		}
	}
	if typ == "path" {
		return filepath.Base(source)
	}
	return "artifact"
}

func terminalProtocol(cfg *object.RenderConfig) string {
	protocol := "auto"
	if cfg != nil && strings.TrimSpace(cfg.TerminalProtocol) != "" {
		protocol = strings.ToLower(strings.TrimSpace(cfg.TerminalProtocol))
	}
	if protocol != "auto" {
		if protocol == "none" {
			return ""
		}
		return protocol
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(strings.ToLower(os.Getenv("TERM")), "kitty") {
		return "kitty"
	}
	if strings.EqualFold(os.Getenv("TERM_PROGRAM"), "iTerm.app") {
		return "iterm"
	}
	if strings.Contains(strings.ToLower(os.Getenv("TERM")), "sixel") {
		return "sixel"
	}
	return ""
}

func kittyImage(res *ResolvedArtifact) string {
	data := strings.TrimPrefix(res.DataURL, "data:"+res.MIME+";base64,")
	return "\033_Gf=100,a=T,m=0;" + data + "\033\\"
}

func itermImage(res *ResolvedArtifact) string {
	data := strings.TrimPrefix(res.DataURL, "data:"+res.MIME+";base64,")
	name := base64.StdEncoding.EncodeToString([]byte(res.Name))
	return fmt.Sprintf("\033]1337;File=name=%s;inline=1:%s\a", name, data)
}

func stripHTML(input string) string {
	var out strings.Builder
	inTag := false
	space := false
	for _, r := range input {
		switch r {
		case '<':
			inTag = true
			if !space {
				out.WriteByte(' ')
				space = true
			}
		case '>':
			inTag = false
		default:
			if inTag {
				continue
			}
			if r == '\n' || r == '\r' || r == '\t' {
				if !space {
					out.WriteByte(' ')
					space = true
				}
				continue
			}
			out.WriteRune(r)
			space = false
		}
	}
	return strings.TrimSpace(html.UnescapeString(collapseSpace(out.String())))
}

func collapseSpace(input string) string {
	fields := strings.Fields(input)
	return strings.Join(fields, " ")
}

func ArtifactFromObject(obj object.Object) (*object.RenderArtifact, bool) {
	switch v := obj.(type) {
	case *object.RenderArtifact:
		return v, true
	case *object.FileValue:
		return artifactFromFileValue(v)
	case *object.ImageValue:
		return artifactFromImageValue(v)
	case *object.TableValue:
		return artifactFromTableValue(v)
	default:
		return nil, false
	}
}

func IsRenderable(obj object.Object) bool {
	_, ok := ArtifactFromObject(obj)
	return ok
}

func DataURLFromBytes(mimeType string, data []byte) string {
	var b bytes.Buffer
	b.WriteString("data:")
	b.WriteString(mimeType)
	b.WriteString(";base64,")
	b.WriteString(base64.StdEncoding.EncodeToString(data))
	return b.String()
}

func artifactFromFileValue(file *object.FileValue) (*object.RenderArtifact, bool) {
	if file == nil {
		return nil, false
	}
	mimeType := detectMIME(file.MIME, file.Path, file.Data)
	art := &object.RenderArtifact{
		Kind:      renderKindForMIME(mimeType),
		SourceTyp: "data",
		MIME:      mimeType,
		Name:      file.Name,
	}
	if shouldExposeText(mimeType) {
		art.Source = string(file.Data)
		return art, true
	}
	art.Source = DataURLFromBytes(mimeType, file.Data)
	return art, true
}

func artifactFromImageValue(img *object.ImageValue) (*object.RenderArtifact, bool) {
	if img == nil {
		return nil, false
	}
	data, mimeType, err := encodedImageBytes(img)
	if err != nil {
		return nil, false
	}
	return &object.RenderArtifact{
		Kind:      "image",
		Source:    DataURLFromBytes(mimeType, data),
		SourceTyp: "data",
		MIME:      mimeType,
		Name:      img.Name,
		Width:     img.Width,
		Height:    img.Height,
	}, true
}

func artifactFromTableValue(table *object.TableValue) (*object.RenderArtifact, bool) {
	if table == nil {
		return nil, false
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if len(table.Columns) > 0 {
		_ = writer.Write(table.Columns)
	}
	for _, row := range table.Rows {
		record := make([]string, len(table.Columns))
		for i, col := range table.Columns {
			if row == nil || row[col] == nil || row[col] == object.NULL {
				record[i] = ""
				continue
			}
			record[i] = row[col].Inspect()
		}
		_ = writer.Write(record)
	}
	writer.Flush()
	return &object.RenderArtifact{
		Kind:      "text",
		Source:    buf.String(),
		SourceTyp: "data",
		MIME:      "text/csv",
		Name:      table.Name,
	}, true
}

func renderKindForMIME(mimeType string) string {
	lower := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(lower, "image/"):
		return "image"
	case strings.Contains(lower, "html"):
		return "html"
	case shouldExposeText(lower):
		return "text"
	default:
		return "file"
	}
}

func encodedImageBytes(img *object.ImageValue) ([]byte, string, error) {
	if img == nil {
		return nil, "", fmt.Errorf("image is null")
	}
	if len(img.Data) > 0 && strings.TrimSpace(img.MIME) != "" {
		return img.Data, img.MIME, nil
	}
	if img.Image == nil {
		return nil, "", fmt.Errorf("image data unavailable")
	}
	format := strings.ToLower(strings.TrimSpace(img.Format))
	mimeType := strings.ToLower(strings.TrimSpace(img.MIME))
	if format == "" {
		switch mimeType {
		case "image/jpeg", "image/jpg":
			format = "jpeg"
		default:
			format = "png"
		}
	}
	var buf bytes.Buffer
	switch format {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, img.Image, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/jpeg", nil
	default:
		if err := png.Encode(&buf, img.Image); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/png", nil
	}
}
