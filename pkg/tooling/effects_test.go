package tooling

import "testing"

func findFinding(t *testing.T, report EffectsReport, capability string) EffectFinding {
	t.Helper()
	for _, f := range report.Capabilities {
		if f.Capability == capability {
			return f
		}
	}
	t.Fatalf("expected capability %q in report, got %+v", capability, report.Capabilities)
	return EffectFinding{}
}

func TestAnalyzeEffectsFindsNetworkFilesystemExecAndDynamicImport(t *testing.T) {
	src := `import "native/os" as os;

let a = http_get("http://example.com");
write_file("out.txt", "data");
os.run("ls", "-la");

let modname = "./" + "helper";
import modname;
`
	report := AnalyzeEffects("sample.spl", src)
	if !report.OK {
		t.Fatalf("expected OK report, got diagnostics: %+v", report.Diagnostics)
	}

	net := findFinding(t, report, "network")
	if len(net.Usages) != 1 || net.Usages[0].Builtin != "http_get" || net.Usages[0].Line != 3 {
		t.Fatalf("unexpected network usages: %+v", net.Usages)
	}

	fw := findFinding(t, report, "filesystem_write")
	if len(fw.Usages) != 1 || fw.Usages[0].Builtin != "write_file" || fw.Usages[0].Line != 4 {
		t.Fatalf("unexpected filesystem_write usages: %+v", fw.Usages)
	}

	exec := findFinding(t, report, "exec")
	if len(exec.Usages) != 1 || exec.Usages[0].Builtin != "native/os.run" || exec.Usages[0].Line != 5 {
		t.Fatalf("unexpected exec usages: %+v", exec.Usages)
	}

	dyn := findFinding(t, report, DynamicImportCapability)
	if len(dyn.Usages) != 1 || dyn.Usages[0].Line != 8 {
		t.Fatalf("unexpected dynamic-import usages: %+v", dyn.Usages)
	}
}

func TestAnalyzeEffectsModuleDotCallResolution(t *testing.T) {
	src := `import "pdf" as pdf;
pdf.to_docx("a.pdf", "a.docx");
`
	report := AnalyzeEffects("sample.spl", src)
	fr := findFinding(t, report, "filesystem_read")
	if len(fr.Usages) != 1 || fr.Usages[0].Builtin != "pdf_to_docx" {
		t.Fatalf("unexpected filesystem_read usages: %+v", fr.Usages)
	}
	fw := findFinding(t, report, "filesystem_write")
	if len(fw.Usages) != 1 || fw.Usages[0].Builtin != "pdf_to_docx" {
		t.Fatalf("unexpected filesystem_write usages: %+v", fw.Usages)
	}
}

func TestAnalyzeEffectsNoCapabilitiesForPlainScript(t *testing.T) {
	report := AnalyzeEffects("sample.spl", `let x = 1 + 2; print x;`)
	if !report.OK {
		t.Fatalf("expected OK report")
	}
	if len(report.Capabilities) != 0 {
		t.Fatalf("expected no capability findings, got %+v", report.Capabilities)
	}
}

func TestAnalyzeEffectsReportsParseErrors(t *testing.T) {
	report := AnalyzeEffects("sample.spl", "let x = ;")
	if report.OK {
		t.Fatalf("expected parse failure")
	}
	if len(report.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for parse error")
	}
}

func TestAnalyzeEffectsDeduplicatesRepeatedCalls(t *testing.T) {
	report := AnalyzeEffects("sample.spl", `read_file("a.txt");
read_file("a.txt");
read_file("b.txt");
`)
	fr := findFinding(t, report, "filesystem_read")
	if len(fr.Usages) != 3 {
		t.Fatalf("expected 3 distinct (builtin,line,col) usages, got %+v", fr.Usages)
	}
}
