package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
)

const wordNamespaces = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

// docxContentTypes adalah [Content_Types].xml minimal untuk DOCX.
const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`

const docxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`

const docxDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`

// docxStyles memberi gaya Normal (Calibri 11pt), Heading 1-3, dan ListParagraph.
const docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles ` + wordNamespaces + `>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/>` +
	`<w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:spacing w:before="360" w:after="120"/><w:outlineLvl w:val="0"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="40"/><w:szCs w:val="40"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:spacing w:before="280" w:after="100"/><w:outlineLvl w:val="1"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="30"/><w:szCs w:val="30"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:spacing w:before="240" w:after="80"/><w:outlineLvl w:val="2"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:spacing w:after="60"/><w:ind w:left="720"/></w:pPr></w:style>` +
	`</w:styles>`

// docxSection menetapkan ukuran A4 dengan margin 2 cm / 2,2 cm.
const docxSection = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
	`<w:pgMar w:top="1134" w:right="1247" w:bottom="1134" w:left="1247" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr>`

// xmlEscape meng-escape teks agar aman di dalam XML.
func xmlEscape(text string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(text)); err != nil {
		return text
	}
	return buf.String()
}

// docxParagraph menyusun satu <w:p> dari teks dan gayanya.
func docxParagraph(text, style string) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	if style != "" {
		b.WriteString(`<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`)
	}
	b.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r>`)
	b.WriteString("</w:p>")
	return b.String()
}

// paragraphsToDocx membangun berkas DOCX utuh dari paragraf hasil ekstraksi PDF.
func paragraphsToDocx(paragraphs []pdfParagraph) ([]byte, error) {
	var body strings.Builder
	for _, paragraph := range paragraphs {
		switch {
		case paragraph.bullet:
			body.WriteString(docxParagraph("• "+paragraph.text, "ListParagraph"))
		case paragraph.level == 1:
			body.WriteString(docxParagraph(paragraph.text, "Heading1"))
		case paragraph.level == 2:
			body.WriteString(docxParagraph(paragraph.text, "Heading2"))
		case paragraph.level >= 3:
			body.WriteString(docxParagraph(paragraph.text, "Heading3"))
		default:
			body.WriteString(docxParagraph(paragraph.text, ""))
		}
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document ` + wordNamespaces + `><w:body>` + body.String() + docxSection + `</w:body></w:document>`

	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	parts := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", docxContentTypes},
		{"_rels/.rels", docxRootRels},
		{"word/_rels/document.xml.rels", docxDocumentRels},
		{"word/styles.xml", docxStyles},
		{"word/document.xml", document},
	}
	for _, part := range parts {
		writer, err := archive.Create(part.name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(part.content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// pdfToDocx mengekstrak PDF lalu menyusun DOCX berisi paragraf dan heading asli,
// bukan kotak teks berkoordinat seperti hasil impor PDF.
func pdfToDocx(src []byte) ([]byte, error) {
	paragraphs, err := pdfParagraphs(src)
	if err != nil {
		return nil, err
	}
	return paragraphsToDocx(paragraphs)
}
