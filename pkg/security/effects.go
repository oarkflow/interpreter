package security

import "sort"

// BuiltinCapabilities maps a registered SPL builtin name — the flat name
// under which it is reachable via eval.RegisterBuiltins /
// eval.RegisterPluginBuiltins, i.e. the name that remains after any
// module-import prefix is stripped (see StdModulePrefixes below) — to the
// security Capability* constant(s) it may exercise when called at runtime.
//
// This is the data source for the static "capability/effects" analysis in
// pkg/tooling (AnalyzeEffects) and the `spltool check --effects` CLI flag:
// it lets a host answer "what might this script try to do" WITHOUT running
// it, by matching call expressions in the parsed AST against this table.
//
// The table was built by grepping every call site of
// security.CheckCapabilityAllowed(security.Capability*) and of the
// capability-implying wrapper helpers (CheckExecAllowed, CheckNetworkAllowed,
// CheckDBAllowed, CheckFileReadAllowed, CheckFileWriteAllowed,
// EnvReadAllowed, EnvWriteAllowed, ExitAllowed) across pkg/builtins/**,
// pkg/render, pkg/builtins/tools, pkg/builtins/scheduler,
// pkg/builtins/watcher and plugins/** (pdf, database, integrations, ip,
// emailvalidator, secretr, rules, tcpguard, server), then attributing each
// check to the enclosing registered builtin name(s).
//
// It is NOT guaranteed exhaustive — a builtin that reaches a capability
// check only through several layers of indirection may have been missed,
// and this map must be extended whenever a new builtin gains a capability
// check (or a new capability constant is introduced; see the
// TestAllCapabilitiesAreRegistered consistency check in effects_test.go,
// which fails the build if a capability constant has zero entries here).
var BuiltinCapabilities = map[string][]string{
	// --- exec ---
	"exec": {CapabilityExec},
	// The native/os module's members are not registered as flat global
	// builtins at all (see RegisterStdModule("native/os", ...) in
	// presets_plugins.go) - they're reachable only as os.run(...) etc.
	// after `import "native/os" as os;`. ResolveModuleBuiltinName gives
	// them the synthetic "native/os.<member>" key below instead of the
	// bare member name, so a generic word like "list" from some other
	// (unrelated) module alias can't collide with these.
	"native/os.run":   {CapabilityExec},
	"native/os.which": {CapabilityExec},
	"native/os.list":  {CapabilityExec},
	"media_info":      {CapabilityExec},
	"media_convert":   {CapabilityExec},
	"ffmpeg_install":  {CapabilityExec},

	// --- network ---
	"http_get":            {CapabilityNetwork},
	"http_post":           {CapabilityNetwork},
	"http_request":        {CapabilityNetwork},
	"webhook":             {CapabilityNetwork},
	"smtp_send":           {CapabilityNetwork},
	"ftp_list":            {CapabilityNetwork},
	"ftp_get":             {CapabilityNetwork, CapabilityFilesystemWrite},
	"ftp_put":             {CapabilityNetwork, CapabilityFilesystemRead},
	"sftp_list":           {CapabilityNetwork},
	"sftp_get":            {CapabilityNetwork, CapabilityFilesystemWrite},
	"sftp_put":            {CapabilityNetwork, CapabilityFilesystemRead},
	"dns_lookup":          {CapabilityNetwork},
	"tcp_check":           {CapabilityNetwork},
	"http_probe":          {CapabilityNetwork},
	"pdf_from_url":        {CapabilityNetwork, CapabilityFilesystemWrite},
	"ip_geo_init":         {CapabilityNetwork, CapabilityFilesystemWrite},
	"email_validate":      {CapabilityNetwork},
	"email_validate_bulk": {CapabilityNetwork},
	"render":              {CapabilityNetwork}, // std/render can resolve a remote URL source
	"file":                {CapabilityNetwork},
	"image":               {CapabilityNetwork},

	// --- db ---
	"db_connect":  {CapabilityDB},
	"db_query":    {CapabilityDB},
	"db_exec":     {CapabilityDB},
	"db_begin":    {CapabilityDB},
	"db_commit":   {CapabilityDB},
	"db_rollback": {CapabilityDB},
	"db_tables":   {CapabilityDB},
	"db_close":    {CapabilityDB},
	"query":       {CapabilityDB},
	"lazy_query":  {CapabilityDB},
	"xql_run":     {CapabilityDB},
	"xql_connect": {CapabilityDB},

	// --- filesystem_read ---
	"read_file":       {CapabilityFilesystemRead},
	"file_exists":     {CapabilityFilesystemRead},
	"readdir":         {CapabilityFilesystemRead},
	"glob":            {CapabilityFilesystemRead},
	"stat":            {CapabilityFilesystemRead},
	"read_json":       {CapabilityFilesystemRead},
	"read_csv":        {CapabilityFilesystemRead},
	"file_load":       {CapabilityFilesystemRead},
	"config_load":     {CapabilityFilesystemRead},
	"office_text":     {CapabilityFilesystemRead},
	"office_read":     {CapabilityFilesystemRead},
	"file_search":     {CapabilityFilesystemRead},
	"file_locate":     {CapabilityFilesystemRead},
	"file_finder":     {CapabilityFilesystemRead},
	"file_checksum":   {CapabilityFilesystemRead},
	"archive_list":    {CapabilityFilesystemRead},
	"image_info_file": {CapabilityFilesystemRead},
	"tcpguard_load":   {CapabilityFilesystemRead},
	"secretr_scan":    {CapabilityFilesystemRead},

	// --- filesystem_write ---
	"write_file":          {CapabilityFilesystemWrite},
	"remove_file":         {CapabilityFilesystemWrite},
	"mkdir":               {CapabilityFilesystemWrite},
	"rmdir":               {CapabilityFilesystemWrite},
	"chmod":               {CapabilityFilesystemWrite},
	"write_json":          {CapabilityFilesystemWrite},
	"write_csv":           {CapabilityFilesystemWrite},
	"file_save":           {CapabilityFilesystemWrite},
	"file_encrypt":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_decrypt":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_copy":           {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_move":           {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_rename":         {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"bulk_rename":         {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_move_plan":      {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_copy_plan":      {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_dedupe":         {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_remove_plan":    {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"file_organize":       {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"archive_compress":    {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"archive_extract":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_convert_batch": {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_optimize":      {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_crop_file":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_resize_file":   {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_thumbnail":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_resize":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_convert":       {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"image_save":          {CapabilityFilesystemWrite},
	"image_load":          {CapabilityFilesystemRead},
	"image_render":        {CapabilityFilesystemRead},

	// pdf_* — nearly every PDF operation reads an input path and/or writes
	// an output path via the plugin's shared checkRead/checkWrite helpers.
	"pdf_info":             {CapabilityFilesystemRead},
	"pdf_validate":         {CapabilityFilesystemRead},
	"pdf_merge":            {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_split":            {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_delete_pages":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_reorder":          {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_rotate":           {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_compress":         {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_protect":          {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_decrypt":          {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_watermark":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_add_page_numbers": {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_set_metadata":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_stamp_image":      {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_to_text":          {CapabilityFilesystemRead},
	"pdf_to_html":          {CapabilityFilesystemRead},
	"pdf_to_markdown":      {CapabilityFilesystemRead},
	"pdf_to_docx":          {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_markdown_to_docx": {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_from_docx":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_docx_to_markdown": {CapabilityFilesystemRead},
	"pdf_docx_to_text":     {CapabilityFilesystemRead},
	"pdf_docx_to_html":     {CapabilityFilesystemRead},
	"pdf_html_to_docx":     {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_to_json":          {CapabilityFilesystemRead},
	"pdf_search":           {CapabilityFilesystemRead},
	"pdf_extract_images":   {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_from_html":        {CapabilityFilesystemWrite},
	"pdf_from_markdown":    {CapabilityFilesystemWrite},
	"pdf_images_to_pdf":    {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_fill_form":        {CapabilityFilesystemRead, CapabilityFilesystemWrite},
	"pdf_list_form_fields": {CapabilityFilesystemRead},
	"pdf_quick":            {CapabilityFilesystemRead, CapabilityFilesystemWrite},

	// --- secrets ---
	"secretr_get":    {CapabilitySecrets},
	"secretr_set":    {CapabilitySecrets},
	"secretr_delete": {CapabilitySecrets},
	"secretr_list":   {CapabilitySecrets},
	// secretr_scan also touches the filesystem; see filesystem_read above.

	// --- policy ---
	"rules_service": {CapabilityPolicy},
	"tcpguard_new":  {CapabilityPolicy},
	"permissions":   {CapabilityPolicy},

	// --- server ---
	"server":       {CapabilityServer},
	"listen":       {CapabilityServer, CapabilityNetwork},
	"listen_async": {CapabilityServer, CapabilityNetwork},

	// --- system ---
	"system_info": {CapabilitySystem},

	// --- scheduler ---
	"schedule":          {CapabilityScheduler},
	"schedule_once":     {CapabilityScheduler},
	"schedule_interval": {CapabilityScheduler},
	"schedule_cancel":   {CapabilityScheduler},
	"schedule_list":     {CapabilityScheduler},
	"schedule_persist":  {CapabilityScheduler},
	"schedule_restore":  {CapabilityScheduler},
	"schedule_now":      {CapabilityScheduler},
	"schedule_run":      {CapabilityScheduler},
	"schedule_worker":   {CapabilityScheduler},
	"schedule_timezone": {CapabilityScheduler},

	// --- async ---
	"go":         {CapabilityAsync},
	"go_async":   {CapabilityAsync},
	"background": {CapabilityAsync},

	// --- watch ---
	"watch":      {CapabilityWatch},
	"hot_reload": {CapabilityWatch},

	// --- process_exit ---
	"exit": {CapabilityProcessExit},

	// --- env_read / env_write ---
	"os_env": {CapabilityEnvRead, CapabilityEnvWrite},
}

// StdModulePrefixes mirrors the prefix argument passed to
// RegisterStdBuiltinModuleWithPrefix in the root package's
// presets_plugins.go: for a module imported as `import "path" as alias`, a
// call `alias.method(...)` resolves at runtime to the builtin registered as
// prefix+method. A module not listed here uses an empty prefix, i.e. the
// module member name IS already the flat builtin name (e.g. `fs.read_file`
// resolves to the builtin "read_file").
//
// Keep this in sync with presets_plugins.go's init(); it only needs entries
// for modules whose members are capability-relevant.
var StdModulePrefixes = map[string]string{
	"database":       "db_",
	"images":         "image_",
	"xql":            "xql_",
	"lua":            "lua_",
	"securetoken":    "securetoken_",
	"naturaldate":    "naturaldate_",
	"wuid":           "wuid_",
	"money":          "money_",
	"phone":          "phone_",
	"ip":             "ip_",
	"shamir":         "shamir_",
	"yaml":           "yaml_",
	"config/yaml":    "yaml_",
	"pdf":            "pdf_",
	"rules":          "rules_",
	"secretr":        "secretr_",
	"tcpguard":       "tcpguard_",
	"emailvalidator": "email_",
}

// ResolveModuleBuiltinName returns the registry key that `alias.member(...)`
// resolves to, given the import path bound to alias (e.g.
// ResolveModuleBuiltinName("pdf", "to_docx") == "pdf_to_docx"). For ordinary
// std modules this is the real flat builtin name (prefix+member, or bare
// member when the module has no prefix - see StdModulePrefixes). The
// "native/os" module is a special case: its members have no flat global
// builtin name of their own, so it gets a synthetic "native/os.<member>"
// key instead, to avoid a generic member name (e.g. "list") colliding with
// an unrelated module's or the top level's builtin of the same name.
func ResolveModuleBuiltinName(modulePath, member string) string {
	if modulePath == "native/os" {
		return "native/os." + member
	}
	if prefix, ok := StdModulePrefixes[modulePath]; ok {
		return prefix + member
	}
	return member
}

// CapabilitiesForBuiltin returns a copy of the capability constants
// associated with builtin name, or nil if none are known.
func CapabilitiesForBuiltin(name string) []string {
	caps := BuiltinCapabilities[name]
	if len(caps) == 0 {
		return nil
	}
	out := make([]string, len(caps))
	copy(out, caps)
	return out
}

// AllCapabilities returns every Capability* constant defined in this
// package, sorted, for use by consistency checks and reporting.
func AllCapabilities() []string {
	caps := []string{
		CapabilityAsync,
		CapabilityDB,
		CapabilityEnvRead,
		CapabilityEnvWrite,
		CapabilityExec,
		CapabilityFilesystemRead,
		CapabilityFilesystemWrite,
		CapabilityNetwork,
		CapabilityPolicy,
		CapabilityProcessExit,
		CapabilityScheduler,
		CapabilityServer,
		CapabilitySecrets,
		CapabilitySystem,
		CapabilityWatch,
	}
	sort.Strings(caps)
	return caps
}
