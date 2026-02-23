package interpreter

import (
	"strings"
	"testing"
)

func TestCryptoAndUtilityBuiltins(t *testing.T) {
	hashObj := testEval(`hash("sha256", "abc");`)
	hashStr, ok := hashObj.(*String)
	if !ok {
		t.Fatalf("expected hash result to be String, got %T", hashObj)
	}
	if hashStr.Value != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("unexpected sha256 hash: %s", hashStr.Value)
	}

	decryptObj := testEval(`
		let key = "0123456789abcdef";
		let c = encrypt("aes_gcm", key, "hello");
		decrypt("aes_gcm", key, c);
	`)
	decryptStr, ok := decryptObj.(*String)
	if !ok {
		t.Fatalf("expected decrypt result to be String, got %T", decryptObj)
	}
	if decryptStr.Value != "hello" {
		t.Fatalf("unexpected decrypt output: %s", decryptStr.Value)
	}

	apiKeyObj := testEval(`api_key("sk", 16);`)
	apiKeyStr, ok := apiKeyObj.(*String)
	if !ok {
		t.Fatalf("expected api_key result to be String, got %T", apiKeyObj)
	}
	if !strings.HasPrefix(apiKeyStr.Value, "sk_") {
		t.Fatalf("api_key prefix mismatch: %s", apiKeyStr.Value)
	}

	testBooleanObject(t, testEval(`
		let h = password_hash("s3cr3t");
		password_verify("s3cr3t", h);
	`), true)
	testBooleanObject(t, testEval(`
		let h = password_hash("s3cr3t");
		password_verify("wrong", h);
	`), false)
}

func TestCollectionBuiltins(t *testing.T) {
	testIntegerObject(t, testEval(`len(range(5));`), 5)
	testIntegerObject(t, testEval(`reduce([1,2,3,4], "sum");`), 10)
	testIntegerObject(t, testEval(`len(uniq([1,1,2,2,3,3]));`), 3)
	testIntegerObject(t, testEval(`find([1,2,3], 2);`), 2)
	testIntegerObject(t, testEval(`
		let s = sort([3, 1, 2]);
		s[0] + s[1] + s[2];
	`), 6)
}
