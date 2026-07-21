# 34 — PDF Generation & Editing

Source: `plugins/pdf` (optional package, wraps `github.com/oarkflow/pdf`;
linked only into `cmd/interpreter`). No dedicated `import` module name
is registered — these builtins are globally available once the package is
linked. Every function sanitizes its paths and checks file
read/write capability; `pdf_from_url` additionally requires the `network`
capability.

## Quick generation

```spl
pdf_quick("Page one: SPL can write a plain-text PDF in a single call.", "page1.pdf");
```

## Reading metadata / validating / extracting text

```spl
let info = pdf_info("page1.pdf"[, password]);
print sprintf("pages=%d encrypted=%t", info.pages, info.encrypted);

let valid = pdf_validate("page1.pdf");
print valid; // {valid: true, path: "...", pages: 1, encrypted: false}

print pdf_to_text("page1.pdf");
// "Page one: SPL can write a plain-text PDF in a single call."
```

Also: `pdf_to_html`, `pdf_to_markdown`, `pdf_to_json`, `pdf_search(path,
query[, opts])`, `pdf_extract_images(input, outputDir[, password])`,
`pdf_list_form_fields`.

## Generating from Markdown / HTML

```spl
pdf_from_markdown("# Title\n\nHello **world**", "report.pdf", {
    "title": "Demo", "author": "SPL", "theme": "modern", "toc": false
    // theme default: "classic"; margin default 54; page_size default "a4"
});
pdf_from_html("<h1>Hi</h1>", "from_html.pdf");
pdf_from_url("https://example.com", "from_url.pdf"); // requires network capability
pdf_images_to_pdf("album.pdf", ["a.png", "b.png"][, {"size":"a4","image_fit":"contain"}]);
```

## Page operations

```spl
pdf_merge("merged.pdf", "page1.pdf", "report.pdf"); // 2+ inputs
pdf_split("merged.pdf", "first_page.pdf", "1"[, password]);
pdf_delete_pages("merged.pdf", "trimmed.pdf", "2");
pdf_reorder("merged.pdf", "reordered.pdf", "2,1");
pdf_rotate("merged.pdf", "rotated.pdf", "1", 90);
pdf_compress("merged.pdf", "compressed.pdf");
```

Page specs are strings like `"1"`, `"1-3"`, or `"3,2,1"`.

## Security

```spl
pdf_protect("merged.pdf", "protected.pdf", "user-pw", "owner-pw", "aes-128");
// algorithm: rc4-128 | aes-128 (default) | aes-256

print pdf_info("protected.pdf", "user-pw").encrypted; // true

pdf_decrypt("protected.pdf", "decrypted.pdf", "user-pw");
print pdf_info("decrypted.pdf").encrypted; // false
```

## Stamping & metadata

```spl
pdf_watermark("merged.pdf", "watermarked.pdf", "DRAFT", {
    "opacity": 0.25, "angle": 45, "font_size": 60
    // defaults: font_size 48, opacity 0.3, angle 45
});
pdf_add_page_numbers("watermarked.pdf", "numbered.pdf"[, {"format": "Page %d of %d"}]);
pdf_set_metadata("numbered.pdf", "tagged.pdf", {"Title": "Report", "Author": "SPL"});
pdf_stamp_image("tagged.pdf", "stamped.pdf", "logo.png"[, {"x":36,"y":36,"width":100,"height":50}]);
```

## Forms

```spl
let fields = pdf_list_form_fields("form.pdf");
pdf_fill_form("form.pdf", "filled.pdf", {"name": "Alice", "email": "a@example.com"});
```

## Verified end-to-end example

```spl
let page1 = "pdf_demo/page1.pdf";
pdf_quick("Page one: SPL can write a plain-text PDF in a single call.", page1);
print pdf_info(page1).pages; // 1

let md = "pdf_demo/md.pdf";
pdf_from_markdown("# Title\n\nHello **world**", md, {"title": "Demo", "theme": "modern"});
print pdf_info(md).pages; // 1

let merged = "pdf_demo/merged.pdf";
pdf_merge(merged, page1, md);
print pdf_info(merged).pages; // 2

pdf_watermark(merged, "pdf_demo/wm.pdf", "DRAFT", {"opacity": 0.25});
pdf_protect("pdf_demo/wm.pdf", "pdf_demo/protected.pdf", "user-pw", "owner-pw", "aes-128");
print pdf_info("pdf_demo/protected.pdf", "user-pw").encrypted; // true
pdf_decrypt("pdf_demo/protected.pdf", "pdf_demo/decrypted.pdf", "user-pw");
print pdf_info("pdf_demo/decrypted.pdf").encrypted; // false
```

See `examples/pdf_all_in_one.spl` for the full runnable showcase (requires
`cmd/interpreter`).
