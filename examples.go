package interpreter

import "fmt"

// ExampleExecScript runs a simple inline SPL script and prints the result.
func ExampleExecScript() {
	result, err := Exec("let x = 40; let y = 2; x + y;", nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(result.Inspect())
}

// ExampleExecModuleFile runs an SPL file that imports another module.
func ExampleExecModuleFile() {
	result, err := ExecFile("testdata/modules/entry_relative_import.spl", nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(result.Inspect())
}

// ExampleIntegrationHTTPGet shows using integration builtins in an inline SPL script.
func ExampleIntegrationHTTPGet() {
	script := `
let res, err = http_get("https://httpbin.org/get", {"Accept": "application/json"}, 3000);
if (err != null) { return err; }
res.status_code;
`
	result, err := Exec(script, nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(result.Inspect())
}

// ExampleIntegrationSMTPConfig demonstrates smtp_send config shape.
func ExampleIntegrationSMTPConfig() {
	script := `
let ok, err = smtp_send({
  "host": "smtp.example.com",
  "port": 587,
  "username": "user@example.com",
  "password": "secret",
  "from": "noreply@example.com",
  "to": ["alice@example.com"],
  "subject": "SPL demo",
  "body": "Hello"
});
if (err != null) { err; } else { ok; }
`

	fmt.Println(script)
}

// ExampleIntegrationSFTPConfig demonstrates sftp_* config shape.
func ExampleIntegrationSFTPConfig() {
	script := `
let cfg = {
  "host": "sftp.example.com",
  "port": 22,
  "username": "user",
  "password": "secret",
  "timeout_ms": 5000
};
let files, lerr = sftp_list(cfg, "/data");
if (lerr == null) { files; } else { lerr; }
`

	fmt.Println(script)
}
