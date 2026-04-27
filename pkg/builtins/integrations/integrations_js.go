//go:build js

package integrations

import (
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func tupleErr(msg string) object.Object {
	return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
}

func tupleBoolErr(ok bool, msg string) object.Object {
	if ok {
		return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
	}
	return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
}

func unsupportedTupleErr(name string) object.Object {
	return tupleErr(name + " is not supported on js/wasm")
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"http_request": {FnWithEnv: builtinHTTPRequest},
		"http_get":     {FnWithEnv: builtinHTTPGet},
		"http_post":    {FnWithEnv: builtinHTTPPost},
		"webhook":      {FnWithEnv: builtinWebhook},
		"smtp_send":    {Fn: builtinSMTPSend},
		"ftp_list":     {Fn: builtinFTPList},
		"ftp_get":      {Fn: builtinFTPGet},
		"ftp_put":      {Fn: builtinFTPPut},
		"sftp_list":    {Fn: builtinSFTPList},
		"sftp_get":     {Fn: builtinSFTPGet},
		"sftp_put":     {Fn: builtinSFTPPut},
	})
}

func builtinHTTPRequest(env *object.Environment, args ...object.Object) object.Object {
	return unsupportedTupleErr("http_request")
}

func builtinHTTPGet(env *object.Environment, args ...object.Object) object.Object {
	return unsupportedTupleErr("http_get")
}

func builtinHTTPPost(env *object.Environment, args ...object.Object) object.Object {
	return unsupportedTupleErr("http_post")
}

func builtinWebhook(env *object.Environment, args ...object.Object) object.Object {
	return unsupportedTupleErr("webhook")
}

func builtinSMTPSend(args ...object.Object) object.Object {
	return tupleBoolErr(false, "smtp_send is not supported on js/wasm")
}

func builtinFTPList(args ...object.Object) object.Object {
	return unsupportedTupleErr("ftp_list")
}

func builtinFTPGet(args ...object.Object) object.Object {
	return unsupportedTupleErr("ftp_get")
}

func builtinFTPPut(args ...object.Object) object.Object {
	return unsupportedTupleErr("ftp_put")
}

func builtinSFTPList(args ...object.Object) object.Object {
	return unsupportedTupleErr("sftp_list")
}

func builtinSFTPGet(args ...object.Object) object.Object {
	return unsupportedTupleErr("sftp_get")
}

func builtinSFTPPut(args ...object.Object) object.Object {
	return unsupportedTupleErr("sftp_put")
}
