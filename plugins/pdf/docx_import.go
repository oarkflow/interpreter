package pdf

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// This file implements a best-effort DOCX (OOXML WordprocessingML) reader
// used by pdf_docx_to_markdown, pdf_docx_to_text, and pdf_from_docx.
//
// There is no DOCX-parsing library vendored anywhere in this repo or its
// dependencies (github.com/oarkflow/pdf only *writes* DOCX, via
// github.com/oarkflow/pdf/md's DOCX exporter — see docx_export in
// pdf_to_docx's implementation). Rather than shelling out to an external
// converter (LibreOffice, pandoc), which would need an optional external
// binary and cut across this project's sandboxing model, this reconstructs
// Markdown directly from word/document.xml — a DOCX file is just a zip
// archive of XML parts — and then reuses the existing Markdown -> PDF
// pipeline (pdflib.FromMarkdown) to reach PDF.
//
// Scope: paragraph text, heading levels (via pStyle "HeadingN" or built-in
// Word style names like "heading N"), bold/italic/underline runs, bullet
// and numbered lists (via pStyle or the presence of <w:numPr>, using the
// numbering part only to distinguish bullet vs. decimal where possible),
// simple tables, and block quotes/code paragraphs written by our own DOCX
// exporter. This is not a general OOXML layout engine: floating images,
// text boxes, headers/footers, footnotes, track-changes markup, and complex
// nested list numbering are not reconstructed.

// docxDocument is the parsed, in-order content of a DOCX file's body.
type docxDocument struct {
	Blocks []docxBlock
}

type docxBlockKind int

const (
	docxParagraph docxBlockKind = iota
	docxHeading
	docxBulletItem
	docxNumberItem
	docxQuote
	docxCode
	docxTable
)

type docxBlock struct {
	Kind  docxBlockKind
	Level int      // heading level (1-6)
	Text  string   // inline Markdown for paragraph/heading/list/quote/code blocks
	Rows  [][]string // table rows (Kind == docxTable)
}

// numberingKind records whether a numId (from numbering.xml) renders as a
// bulleted or decimal-numbered list, so paragraphs that only carry <w:numPr>
// (no distinguishing pStyle) still come out as the right list type.
type numberingKind struct {
	byNumID map[string]bool // numId -> isBullet
}

func readDocx(path string) (*docxDocument, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer zr.Close()

	var docFile, numberingFile *zip.File
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			docFile = f
		case "word/numbering.xml":
			numberingFile = f
		}
	}
	if docFile == nil {
		return nil, fmt.Errorf("not a DOCX file: missing word/document.xml")
	}

	nk := &numberingKind{byNumID: map[string]bool{}}
	if numberingFile != nil {
		if rc, err := numberingFile.Open(); err == nil {
			parseNumbering(rc, nk)
			rc.Close()
		}
	}

	rc, err := docFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open word/document.xml: %w", err)
	}
	defer rc.Close()

	doc, err := parseDocumentXML(rc, nk)
	if err != nil {
		return nil, fmt.Errorf("parse word/document.xml: %w", err)
	}
	return doc, nil
}

// parseNumbering reads word/numbering.xml just enough to tell, for each
// numId, whether it ultimately resolves to a bullet or a decimal/other
// ordered format. Best-effort: abstractNumId linkage is a common subset of
// the spec, not the full numbering model.
func parseNumbering(r io.Reader, nk *numberingKind) {
	type lvl struct {
		NumFmt struct {
			Val string `xml:"val,attr"`
		} `xml:"numFmt"`
	}
	type abstractNum struct {
		ID  string `xml:"abstractNumId,attr"`
		Lvl []lvl  `xml:"lvl"`
	}
	type numLink struct {
		NumID       string `xml:"numId,attr"`
		AbstractRef struct {
			Val string `xml:"val,attr"`
		} `xml:"abstractNumId"`
	}
	type numbering struct {
		AbstractNums []abstractNum `xml:"abstractNum"`
		Nums         []numLink     `xml:"num"`
	}
	var n numbering
	if err := xml.NewDecoder(r).Decode(&n); err != nil {
		return
	}
	bulletByAbstract := map[string]bool{}
	for _, an := range n.AbstractNums {
		isBullet := false
		if len(an.Lvl) > 0 {
			isBullet = strings.EqualFold(an.Lvl[0].NumFmt.Val, "bullet")
		}
		bulletByAbstract[an.ID] = isBullet
	}
	for _, link := range n.Nums {
		nk.byNumID[link.NumID] = bulletByAbstract[link.AbstractRef.Val]
	}
}

// parseDocumentXML walks word/document.xml as a token stream (rather than
// unmarshaling into a fixed struct) because paragraphs and tables are
// interleaved siblings inside <w:body> and their relative order matters for
// reconstructing readable Markdown.
func parseDocumentXML(r io.Reader, nk *numberingKind) (*docxDocument, error) {
	dec := xml.NewDecoder(r)
	doc := &docxDocument{}
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch localName(se.Name) {
			case "body":
				depth++
			case "p":
				if depth >= 1 {
					block, err := parseParagraph(dec, nk)
					if err != nil {
						return nil, err
					}
					if block != nil {
						doc.Blocks = append(doc.Blocks, *block)
					}
				}
			case "tbl":
				if depth >= 1 {
					rows, err := parseTable(dec)
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						doc.Blocks = append(doc.Blocks, docxBlock{Kind: docxTable, Rows: rows})
					}
				}
			}
		case xml.EndElement:
			if localName(se.Name) == "body" {
				depth--
			}
		}
	}
	return doc, nil
}

// parseParagraph consumes a <w:p>...</w:p> element (the StartElement has
// already been read) and returns the reconstructed block, or nil for an
// empty paragraph (Word documents are full of these as spacing).
func parseParagraph(dec *xml.Decoder, nk *numberingKind) (*docxBlock, error) {
	var (
		styleVal   string
		hasNumPr   bool
		numID      string
		text       strings.Builder
		runBold    bool
		runItalic  bool
		runUnderln bool
		inRun      bool
	)
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch localName(se.Name) {
			case "pStyle":
				styleVal = attrVal(se, "val")
			case "numPr":
				hasNumPr = true
			case "numId":
				numID = attrVal(se, "val")
			case "r":
				inRun = true
				runBold, runItalic, runUnderln = false, false, false
			case "b":
				if inRun {
					runBold = attrVal(se, "val") != "0" && attrVal(se, "val") != "false"
				}
			case "i":
				if inRun {
					runItalic = attrVal(se, "val") != "0" && attrVal(se, "val") != "false"
				}
			case "u":
				if inRun {
					runUnderln = attrVal(se, "val") != "none" && attrVal(se, "val") != ""
				}
			case "t":
				var s string
				if err := dec.DecodeElement(&s, &se); err != nil {
					return nil, err
				}
				text.WriteString(wrapRun(s, runBold, runItalic, runUnderln))
			case "tab":
				text.WriteString("\t")
			case "br", "cr":
				text.WriteString("  \n")
			case "p":
				// Nested paragraph markers (rare); track depth so its `</w:p>`
				// doesn't prematurely end the outer paragraph.
				depth++
			}
		case xml.EndElement:
			switch localName(se.Name) {
			case "r":
				inRun = false
			case "p":
				if depth > 0 {
					depth--
					continue
				}
				return finishParagraph(styleVal, hasNumPr, numID, text.String(), nk), nil
			}
		}
	}
}

func finishParagraph(styleVal string, hasNumPr bool, numID, text string, nk *numberingKind) *docxBlock {
	trimmed := strings.TrimRight(text, " \t")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}
	style := strings.ToLower(styleVal)

	if level, ok := headingLevel(style); ok {
		return &docxBlock{Kind: docxHeading, Level: level, Text: trimmed}
	}
	switch {
	case strings.Contains(style, "quote"):
		return &docxBlock{Kind: docxQuote, Text: trimmed}
	case strings.Contains(style, "code"):
		return &docxBlock{Kind: docxCode, Text: trimmed}
	case strings.Contains(style, "listnumber") || strings.Contains(style, "listparagraphnumber"):
		return &docxBlock{Kind: docxNumberItem, Text: trimmed}
	case strings.Contains(style, "listbullet"):
		return &docxBlock{Kind: docxBulletItem, Text: trimmed}
	case hasNumPr:
		if isBullet, known := nk.byNumID[numID]; known && !isBullet {
			return &docxBlock{Kind: docxNumberItem, Text: trimmed}
		}
		return &docxBlock{Kind: docxBulletItem, Text: trimmed}
	default:
		return &docxBlock{Kind: docxParagraph, Text: trimmed}
	}
}

// headingLevel recognizes both our own exporter's "HeadingN" pStyle values
// and Word's own built-in "heading N" / "Heading N" style names.
func headingLevel(lowerStyle string) (int, bool) {
	if !strings.Contains(lowerStyle, "heading") {
		return 0, false
	}
	digits := strings.TrimLeft(strings.TrimPrefix(lowerStyle, "heading"), " ")
	if digits == "" {
		return 1, true
	}
	if n, err := strconv.Atoi(digits); err == nil && n >= 1 && n <= 6 {
		return n, true
	}
	return 1, true
}

func wrapRun(s string, bold, italic, underline bool) string {
	if s == "" {
		return s
	}
	// Markdown has no native underline; render it as emphasis-adjacent bold
	// so the information isn't silently dropped, without inventing HTML.
	if underline && !bold && !italic {
		bold = true
	}
	switch {
	case bold && italic:
		return "***" + s + "***"
	case bold:
		return "**" + s + "**"
	case italic:
		return "*" + s + "*"
	default:
		return s
	}
}

// parseTable consumes a <w:tbl>...</w:tbl> element (StartElement already
// read) and returns its rows of plain-text cells.
func parseTable(dec *xml.Decoder) ([][]string, error) {
	var rows [][]string
	var row []string
	var cellText strings.Builder
	inCell := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch localName(se.Name) {
			case "tr":
				row = nil
			case "tc":
				inCell = true
				cellText.Reset()
			case "t":
				if inCell {
					var s string
					if err := dec.DecodeElement(&s, &se); err != nil {
						return nil, err
					}
					if cellText.Len() > 0 {
						cellText.WriteString(" ")
					}
					cellText.WriteString(s)
				}
			}
		case xml.EndElement:
			switch localName(se.Name) {
			case "tc":
				inCell = false
				row = append(row, strings.TrimSpace(cellText.String()))
			case "tr":
				rows = append(rows, row)
			case "tbl":
				return rows, nil
			}
		}
	}
}

func localName(name xml.Name) string {
	if idx := strings.LastIndexByte(name.Local, ':'); idx >= 0 {
		return name.Local[idx+1:]
	}
	return name.Local
}

func attrVal(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if localName(a.Name) == local {
			return a.Value
		}
	}
	return ""
}

// renderMarkdown turns the parsed document into Markdown text.
func (d *docxDocument) renderMarkdown() string {
	var b strings.Builder
	prevList := docxBlockKind(-1)
	for i, blk := range d.Blocks {
		if i > 0 {
			b.WriteString("\n")
			// Consecutive list items of the same kind don't need a full
			// blank-line paragraph break between them.
			if !(blk.Kind == prevList && (blk.Kind == docxBulletItem || blk.Kind == docxNumberItem)) {
				b.WriteString("\n")
			}
		}
		switch blk.Kind {
		case docxHeading:
			b.WriteString(strings.Repeat("#", blk.Level))
			b.WriteString(" ")
			b.WriteString(blk.Text)
		case docxQuote:
			b.WriteString("> ")
			b.WriteString(blk.Text)
		case docxCode:
			b.WriteString("    ")
			b.WriteString(blk.Text)
		case docxBulletItem:
			b.WriteString("- ")
			b.WriteString(blk.Text)
		case docxNumberItem:
			b.WriteString("1. ")
			b.WriteString(blk.Text)
		case docxTable:
			b.WriteString(renderMarkdownTable(blk.Rows))
		default:
			b.WriteString(blk.Text)
		}
		prevList = blk.Kind
	}
	return b.String()
}

func renderMarkdownTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	pad := func(row []string) []string {
		for len(row) < cols {
			row = append(row, "")
		}
		return row
	}
	esc := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	var b strings.Builder
	header := pad(rows[0])
	b.WriteString("|")
	for _, c := range header {
		b.WriteString(" ")
		b.WriteString(esc(c))
		b.WriteString(" |")
	}
	b.WriteString("\n|")
	for range header {
		b.WriteString(" --- |")
	}
	for _, row := range rows[1:] {
		row = pad(row)
		b.WriteString("\n|")
		for _, c := range row {
			b.WriteString(" ")
			b.WriteString(esc(c))
			b.WriteString(" |")
		}
	}
	return b.String()
}

// renderPlainText turns the parsed document into plain text (Markdown
// syntax stripped to the extent it was added by renderMarkdown), for
// pdf_docx_to_text.
func (d *docxDocument) renderPlainText() string {
	var b strings.Builder
	for i, blk := range d.Blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		switch blk.Kind {
		case docxTable:
			for _, row := range blk.Rows {
				b.WriteString(strings.Join(row, "\t"))
				b.WriteString("\n")
			}
		default:
			b.WriteString(stripMarkdownEmphasis(blk.Text))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func stripMarkdownEmphasis(s string) string {
	for _, marker := range []string{"***", "**", "*"} {
		s = strings.ReplaceAll(s, marker, "")
	}
	return s
}
