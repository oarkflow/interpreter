# SPL Builtin Function Reference

> This file is generated — do not hand-edit; regenerate with `make builtins-doc`
> (generator: `cmd/spltool-full/cmd/builtindocs`).

Two kinds of builtins exist. **Global builtins** resolve as bare identifiers in any
script, no import required. **Plugin builtins** are only reachable after importing
their owning std module (`import "pdf" as pdf;`), and are called through that
module namespace (`pdf.to_docx(...)`).

## Global builtins

326 global builtins (no import required).

- **`E`** — (undocumented)
- **`Error`** — Error(message[, details]) returns structured error object with message, code, stack
- **`INF`** — (undocumented)
- **`NAN`** — (undocumented)
- **`PI`** — (undocumented)
- **`abs`** — (undocumented)
- **`acos`** — (undocumented)
- **`add_months`** — (undocumented)
- **`all`** — (undocumented)
- **`any`** — (undocumented)
- **`api_key`** — (undocumented)
- **`asin`** — (undocumented)
- **`assert_contains`** — assert_contains(haystack, needle[, message]) fails test when needle not found in haystack string or array
- **`assert_eq`** — assert_eq(actual, expected[, message]) fails test when values differ
- **`assert_neq`** — assert_neq(actual, unexpected[, message]) fails test when values are equal
- **`assert_throws`** — assert_throws(fn[, message]) fails test when fn does not produce an error
- **`assert_true`** — assert_true(condition[, message]) fails test when condition is false
- **`atan`** — (undocumented)
- **`atan2`** — (undocumented)
- **`avg`** — (undocumented)
- **`await_all`** — (undocumented)
- **`await_race`** — (undocumented)
- **`background`** — (undocumented)
- **`base64_decode`** — (undocumented)
- **`base64_encode`** — (undocumented)
- **`basename`** — (undocumented)
- **`batch`** — (undocumented)
- **`camel_case`** — (undocumented)
- **`cbrt`** — (undocumented)
- **`ceil`** — (undocumented)
- **`channel`** — channel([buffer_size]) creates a message channel
- **`chars`** — (undocumented)
- **`chmod`** — (undocumented)
- **`chunk`** — (undocumented)
- **`clamp`** — (undocumented)
- **`coalesce`** — (undocumented)
- **`compact`** — (undocumented)
- **`computed`** — (undocumented)
- **`config_load`** — config_load(path[, format]) loads JSON/YAML/.env config and wraps secret-like fields
- **`config_parse`** — config_parse(raw, format) parses JSON/YAML/.env string and wraps secret-like fields
- **`constant_time_eq`** — (undocumented)
- **`contains`** — (undocumented)
- **`cos`** — (undocumented)
- **`cosh`** — (undocumented)
- **`count_substr`** — (undocumented)
- **`csv_decode`** — csv_decode(text[, opts]) decodes CSV text into TABLE_VALUE
- **`csv_encode`** — csv_encode(table_or_rows[, opts]) encodes rows as CSV text
- **`date_with_format`** — (undocumented)
- **`decrypt`** — (undocumented)
- **`deep_equal`** — (undocumented)
- **`default`** — (undocumented)
- **`delete_key`** — (undocumented)
- **`dirname`** — (undocumented)
- **`drop`** — (undocumented)
- **`effect`** — (undocumented)
- **`encrypt`** — (undocumented)
- **`end_of_day`** — (undocumented)
- **`end_of_month`** — (undocumented)
- **`end_of_week`** — (undocumented)
- **`ends_with`** — (undocumented)
- **`entries`** — (undocumented)
- **`escape_html`** — (undocumented)
- **`exec`** — exec(command, ...args[, timeout_ms]) runs a whitelisted OS command; disabled by SPL_DISABLE_EXEC or host protection
- **`exit`** — (undocumented)
- **`exp`** — (undocumented)
- **`factorial`** — (undocumented)
- **`file`** — file(path_or_url_or_data[, opts]) creates a renderable file artifact
- **`file_bytes`** — file_bytes(file_value) returns file content as base64 STRING
- **`file_copy`** — file_copy(src, dst) copies a file path or FILE_VALUE-backed path
- **`file_exists`** — (undocumented)
- **`file_load`** — file_load(path_or_artifact[, opts]) loads content into FILE_VALUE
- **`file_mime`** — file_mime(file_value) returns the MIME type
- **`file_move`** — file_move(src, dst) moves a file path or FILE_VALUE-backed path
- **`file_name`** — file_name(file_value) returns the file name
- **`file_rename`** — file_rename(path, new_name) renames a file in place
- **`file_save`** — file_save(file_value, path[, opts]) writes FILE_VALUE content to disk
- **`file_size`** — file_size(file_value) returns the file size in bytes
- **`file_text`** — file_text(file_value) returns file content as STRING
- **`find`** — (undocumented)
- **`first`** — (undocumented)
- **`flatten`** — (undocumented)
- **`floor`** — (undocumented)
- **`format_duration`** — (undocumented)
- **`format_time`** — (undocumented)
- **`format_time_tz`** — (undocumented)
- **`from_entries`** — (undocumented)
- **`gcd`** — (undocumented)
- **`generator`** — generator(fn) wraps function result as lazy iterable
- **`get`** — (undocumented)
- **`glob`** — (undocumented)
- **`go`** — go(fn[, ...args]) runs function asynchronously and returns future
- **`go_async`** — (undocumented)
- **`group_by`** — (undocumented)
- **`has_key`** — (undocumented)
- **`hash`** — (undocumented)
- **`help`** — help() lists builtin names; help("name") shows details for one builtin
- **`hex_decode`** — (undocumented)
- **`hex_encode`** — (undocumented)
- **`hmac`** — (undocumented)
- **`hmac_sha256`** — (undocumented)
- **`hmac_sha512`** — (undocumented)
- **`hot_reload`** — (undocumented)
- **`hypot`** — (undocumented)
- **`image`** — image(path_or_url_or_data[, opts]) creates a renderable image artifact
- **`immutable`** — immutable(value) returns deeply frozen copy
- **`index_by`** — (undocumented)
- **`index_of`** — (undocumented)
- **`input`** — (undocumented)
- **`interpolate`** — interpolate(template, data[, ...positional]) replaces {key} or {index} placeholders
- **`is_alnum`** — (undocumented)
- **`is_alpha`** — (undocumented)
- **`is_array`** — (undocumented)
- **`is_blank`** — (undocumented)
- **`is_bool`** — (undocumented)
- **`is_even`** — (undocumented)
- **`is_finite`** — (undocumented)
- **`is_float`** — (undocumented)
- **`is_function`** — (undocumented)
- **`is_hash`** — (undocumented)
- **`is_inf`** — (undocumented)
- **`is_int`** — (undocumented)
- **`is_integer`** — (undocumented)
- **`is_nan`** — (undocumented)
- **`is_null`** — (undocumented)
- **`is_number`** — (undocumented)
- **`is_numeric`** — (undocumented)
- **`is_odd`** — (undocumented)
- **`is_prime`** — (undocumented)
- **`is_string`** — (undocumented)
- **`is_weekend`** — (undocumented)
- **`iso_to_unix`** — (undocumented)
- **`iso_to_unix_ms`** — (undocumented)
- **`join`** — (undocumented)
- **`json_decode`** — (undocumented)
- **`json_encode`** — (undocumented)
- **`json_parse`** — (undocumented)
- **`json_stringify`** — (undocumented)
- **`kebab_case`** — (undocumented)
- **`keys`** — (undocumented)
- **`last`** — (undocumented)
- **`last_index_of`** — (undocumented)
- **`lcm`** — (undocumented)
- **`len`** — (undocumented)
- **`lerp`** — (undocumented)
- **`log`** — (undocumented)
- **`log10`** — (undocumented)
- **`log2`** — (undocumented)
- **`lower`** — (undocumented)
- **`map_range`** — (undocumented)
- **`max`** — (undocumented)
- **`md5`** — (undocumented)
- **`mean`** — (undocumented)
- **`median`** — (undocumented)
- **`merge`** — (undocumented)
- **`metric`** — metric(name, value[, labels]) records metric point
- **`min`** — (undocumented)
- **`mkdir`** — (undocumented)
- **`mod`** — (undocumented)
- **`mode`** — (undocumented)
- **`month`** — (undocumented)
- **`move`** — move(value) transfers ownership marker to current scope
- **`normalize`** — (undocumented)
- **`now`** — (undocumented)
- **`now_format`** — (undocumented)
- **`now_iso`** — (undocumented)
- **`omit`** — (undocumented)
- **`os_env`** — (undocumented)
- **`pad_left`** — (undocumented)
- **`pad_right`** — (undocumented)
- **`parse_bool`** — (undocumented)
- **`parse_duration`** — (undocumented)
- **`parse_float`** — (undocumented)
- **`parse_int`** — (undocumented)
- **`parse_string`** — (undocumented)
- **`parse_time`** — (undocumented)
- **`parse_time_tz`** — (undocumented)
- **`parse_type`** — (undocumented)
- **`partition`** — (undocumented)
- **`pascal_case`** — (undocumented)
- **`password_generate`** — (undocumented)
- **`password_hash`** — (undocumented)
- **`password_verify`** — (undocumented)
- **`path_abs`** — (undocumented)
- **`path_base`** — (undocumented)
- **`path_clean`** — (undocumented)
- **`path_dir`** — (undocumented)
- **`path_ext`** — (undocumented)
- **`path_join`** — (undocumented)
- **`percent`** — (undocumented)
- **`percentile`** — (undocumented)
- **`permissions`** — permissions(policy_hash) applies runtime allow/deny policy
- **`pick`** — (undocumented)
- **`pluck`** — (undocumented)
- **`pow`** — (undocumented)
- **`printf`** — printf(format, ...args) prints formatted text and returns it
- **`push`** — (undocumented)
- **`puts`** — (undocumented)
- **`random`** — (undocumented)
- **`random_bytes`** — (undocumented)
- **`random_choice`** — (undocumented)
- **`random_float`** — (undocumented)
- **`random_range`** — (undocumented)
- **`random_string`** — (undocumented)
- **`range`** — (undocumented)
- **`read_csv`** — read_csv(path[, opts]) loads CSV into TABLE_VALUE
- **`read_file`** — (undocumented)
- **`read_json`** — read_json(path[, opts]) loads JSON from disk
- **`readdir`** — (undocumented)
- **`recv`** — recv(channel) receives a value from channel
- **`reduce`** — (undocumented)
- **`regex_find_all`** — (undocumented)
- **`regex_match`** — (undocumented)
- **`regex_replace`** — (undocumented)
- **`regex_split`** — (undocumented)
- **`remove_file`** — (undocumented)
- **`render`** — render(value[, opts]) creates or updates a renderable artifact
- **`repeat`** — (undocumented)
- **`replace`** — (undocumented)
- **`replace_n`** — (undocumented)
- **`rest`** — (undocumented)
- **`reverse`** — (undocumented)
- **`reverse_string`** — (undocumented)
- **`rmdir`** — (undocumented)
- **`round`** — (undocumented)
- **`round_to`** — (undocumented)
- **`run_tests`** — run_tests(path_or_paths) executes SPL test scripts and returns summary
- **`sample`** — (undocumented)
- **`schedule`** — (undocumented)
- **`schedule_cancel`** — (undocumented)
- **`schedule_interval`** — (undocumented)
- **`schedule_list`** — (undocumented)
- **`schedule_now`** — (undocumented)
- **`schedule_once`** — (undocumented)
- **`schedule_persist`** — (undocumented)
- **`schedule_restore`** — (undocumented)
- **`schedule_run`** — (undocumented)
- **`schedule_timezone`** — (undocumented)
- **`schedule_worker`** — (undocumented)
- **`secret`** — secret(value) wraps a string as non-displayable secret
- **`secret_mask`** — secret_mask(value[, visible]) returns masked display string
- **`secret_reveal`** — secret_reveal(secret_value) reveals a SECRET as plain STRING
- **`seed_random`** — (undocumented)
- **`send`** — send(channel, value) sends a value to channel
- **`setSignal`** — (undocumented)
- **`sha256`** — (undocumented)
- **`sha512`** — (undocumented)
- **`shuffle`** — (undocumented)
- **`sign`** — (undocumented)
- **`signal`** — (undocumented)
- **`sin`** — (undocumented)
- **`sinh`** — (undocumented)
- **`sleep`** — (undocumented)
- **`slice`** — (undocumented)
- **`slug`** — (undocumented)
- **`snake_case`** — (undocumented)
- **`sort`** — (undocumented)
- **`sort_by`** — (undocumented)
- **`split`** — (undocumented)
- **`split_lines`** — (undocumented)
- **`sprintf`** — sprintf(format, ...args) formats values with printf-style placeholders; supports %T for SPL type
- **`sqrt`** — (undocumented)
- **`start_of_day`** — (undocumented)
- **`start_of_month`** — (undocumented)
- **`start_of_week`** — (undocumented)
- **`starts_with`** — (undocumented)
- **`stat`** — (undocumented)
- **`stddev`** — (undocumented)
- **`stream`** — (undocumented)
- **`stream_collect`** — (undocumented)
- **`stream_filter`** — (undocumented)
- **`stream_map`** — (undocumented)
- **`stream_reduce`** — (undocumented)
- **`stream_to_array`** — (undocumented)
- **`substring`** — (undocumented)
- **`sum`** — (undocumented)
- **`swap_case`** — (undocumented)
- **`table_columns`** — table_columns(table) returns ARRAY of column names
- **`table_filter`** — table_filter(table, fn) filters rows using a callback
- **`table_map`** — table_map(table, fn) maps rows using a callback that returns HASH
- **`table_rows`** — table_rows(table) returns TABLE_VALUE rows as ARRAY of HASH
- **`table_select`** — table_select(table, columns) keeps selected columns
- **`take`** — (undocumented)
- **`tan`** — (undocumented)
- **`tanh`** — (undocumented)
- **`test_summary`** — test_summary() returns {total, passed, failed}
- **`time`** — (undocumented)
- **`time_add`** — (undocumented)
- **`time_diff`** — (undocumented)
- **`time_ms`** — (undocumented)
- **`time_sub`** — (undocumented)
- **`title`** — (undocumented)
- **`to_degrees`** — (undocumented)
- **`to_float`** — (undocumented)
- **`to_int`** — (undocumented)
- **`to_radians`** — (undocumented)
- **`to_string`** — (undocumented)
- **`to_timezone`** — (undocumented)
- **`trace`** — trace(name[, attrs]) emits trace event
- **`trim`** — (undocumented)
- **`trim_chars`** — (undocumented)
- **`trim_prefix`** — (undocumented)
- **`trim_suffix`** — (undocumented)
- **`trunc`** — (undocumented)
- **`truncate`** — (undocumented)
- **`type`** — (undocumented)
- **`typeof`** — (undocumented)
- **`unescape_html`** — (undocumented)
- **`uniq`** — (undocumented)
- **`unique`** — (undocumented)
- **`unix_ms_to_iso`** — (undocumented)
- **`unix_to_iso`** — (undocumented)
- **`unwatch`** — (undocumented)
- **`upper`** — (undocumented)
- **`url_decode`** — (undocumented)
- **`url_encode`** — (undocumented)
- **`uuid`** — uuid([version]) generates UUID, default version is 7; supports 4 or 7
- **`values`** — (undocumented)
- **`variance`** — (undocumented)
- **`watch`** — (undocumented)
- **`weekday`** — (undocumented)
- **`words`** — (undocumented)
- **`write_csv`** — write_csv(path, table_or_rows[, opts]) saves CSV to disk
- **`write_file`** — (undocumented)
- **`write_json`** — write_json(path, value[, opts]) saves JSON to disk
- **`year`** — (undocumented)
- **`zip`** — (undocumented)

## Plugin builtins by module

21 plugin modules, 136 plugin builtins total. Each module is reachable via
`import "<module>" as <alias>;` and its builtins are called as `<alias>.<member>(...)`.

### cryptoextra

- **`bcrypt_hash`** (`cryptoextra.bcrypt_hash`) — bcrypt_hash(password[, cost]) hashes a password with bcrypt (requires cryptoextra plugin)
- **`bcrypt_verify`** (`cryptoextra.bcrypt_verify`) — bcrypt_verify(password, hash) verifies a password against a bcrypt hash (requires cryptoextra plugin)
- **`jwt_decode`** (`cryptoextra.jwt_decode`) — jwt_decode(token, secret[, opts]) verifies and decodes a JWT string into a HASH of claims; raises on invalid signature/alg/expiry (requires cryptoextra plugin)
- **`jwt_encode`** (`cryptoextra.jwt_encode`) — jwt_encode(claims, secret[, opts]) signs a HASH of claims into a JWT string; opts supports alg (HS256/HS384/HS512) and expires_in seconds (requires cryptoextra plugin)

### database

- **`db_begin`** (`database.begin`) — db_begin(db) starts a database transaction
- **`db_close`** (`database.close`) — db_close(db) closes a database connection
- **`db_commit`** (`database.commit`) — db_commit(tx) commits a database transaction
- **`db_connect`** (`database.connect`) — db_connect(driver, connection_string) opens a database connection
- **`db_exec`** (`database.exec`) — db_exec(db_or_tx, query[, params][, timeout_ms]) executes a statement; params may be ARRAY or HASH; optional trailing timeout_ms bounds this call
- **`db_query`** (`database.db_query`) — db_query(db_or_tx, query[, params][, format][, timeout_ms]) runs a query; params may be ARRAY or HASH; format is table or array; optional trailing timeout_ms bounds this call
- **`db_rollback`** (`database.rollback`) — db_rollback(tx) rolls back a database transaction
- **`db_tables`** (`database.tables`) — db_tables(db_or_tx) lists database tables
- **`lazy_query`** (`database.lazy_query`) — (undocumented)
- **`query`** (`database.query`) — (undocumented)

### emailvalidator

- **`email_is_disposable`** (`emailvalidator.is_disposable`) — (undocumented)
- **`email_is_free_provider`** (`emailvalidator.is_free_provider`) — (undocumented)
- **`email_is_role_account`** (`emailvalidator.is_role_account`) — (undocumented)
- **`email_validate`** (`emailvalidator.validate`) — (undocumented)
- **`email_validate_bulk`** (`emailvalidator.validate_bulk`) — (undocumented)
- **`email_validate_syntax`** (`emailvalidator.validate_syntax`) — (undocumented)

### images

- **`image_convert`** (`images.convert`) — image_convert(image_value, format[, opts]) re-encodes an image
- **`image_crop`** (`images.crop`) — image_crop(image_value, x, y, width, height) crops an image
- **`image_info`** (`images.info`) — image_info(image_value) returns metadata for an image
- **`image_load`** (`images.load`) — image_load(path_or_artifact[, opts]) decodes an image into IMAGE_VALUE
- **`image_render`** (`images.render`) — image_render(image_value[, opts]) creates a renderable image artifact
- **`image_resize`** (`images.resize`) — image_resize(image_value, width, height[, opts]) resizes an image
- **`image_rotate`** (`images.rotate`) — image_rotate(image_value, degrees[, opts]) rotates an image

### integrations

- **`ftp_get`** (`integrations.ftp_get`) — ftp_get(config, remote_path, local_path) downloads file over FTP
- **`ftp_list`** (`integrations.ftp_list`) — ftp_list(config, remote_dir) lists directory entries over FTP
- **`ftp_put`** (`integrations.ftp_put`) — ftp_put(config, local_path, remote_path) uploads file over FTP
- **`http_get`** (`integrations.http_get`) — http_get(url[, headers][, timeout_ms]) performs HTTP GET
- **`http_post`** (`integrations.http_post`) — http_post(url, body[, headers][, timeout_ms]) performs HTTP POST
- **`http_request`** (`integrations.http_request`) — http_request(method, url[, body][, headers][, timeout_ms]) performs an HTTP request
- **`sftp_get`** (`integrations.sftp_get`) — sftp_get(config, remote_path, local_path) downloads file over SFTP
- **`sftp_list`** (`integrations.sftp_list`) — sftp_list(config, remote_dir) lists directory entries over SFTP
- **`sftp_put`** (`integrations.sftp_put`) — sftp_put(config, local_path, remote_path) uploads file over SFTP
- **`smtp_send`** (`integrations.smtp_send`) — smtp_send(config) sends email via SMTP
- **`webhook`** (`integrations.webhook`) — webhook(url, payload[, headers][, timeout_ms]) sends a webhook POST

### ip

- **`ip_client_from_header`** (`ip.client_from_header`) — (undocumented)
- **`ip_country`** (`ip.country`) — (undocumented)
- **`ip_geo_init`** (`ip.geo_init`) — (undocumented)
- **`ip_is_private`** (`ip.is_private`) — (undocumented)
- **`ip_lookup`** (`ip.lookup`) — (undocumented)
- **`ip_lookup_bulk`** (`ip.lookup_bulk`) — (undocumented)

### lua

- **`lua_eval`** (`lua.eval`) — (undocumented)
- **`lua_load`** (`lua.load`) — (undocumented)
- **`lua_run`** (`lua.run`) — (undocumented)
- **`lua_version`** (`lua.version`) — (undocumented)

### metadata

- **`infer_csv_types`** (`metadata.infer_csv_types`) — (undocumented)
- **`infer_json_types`** (`metadata.infer_json_types`) — (undocumented)
- **`infer_value_type`** (`metadata.infer_value_type`) — (undocumented)

### money

- **`money_add`** (`money.add`) — (undocumented)
- **`money_format`** (`money.format`) — (undocumented)
- **`money_mul`** (`money.mul`) — (undocumented)
- **`money_new`** (`money.new`) — (undocumented)
- **`money_percent`** (`money.percent`) — (undocumented)
- **`money_sub`** (`money.sub`) — (undocumented)

### naturaldate

- **`naturaldate_parse`** (`naturaldate.parse`) — (undocumented)
- **`naturaldate_parse_all`** (`naturaldate.parse_all`) — (undocumented)

### pdf

- **`pdf_add_page_numbers`** (`pdf.add_page_numbers`) — (undocumented)
- **`pdf_compress`** (`pdf.compress`) — (undocumented)
- **`pdf_decrypt`** (`pdf.decrypt`) — (undocumented)
- **`pdf_delete_pages`** (`pdf.delete_pages`) — (undocumented)
- **`pdf_docx_to_html`** (`pdf.docx_to_html`) — (undocumented)
- **`pdf_docx_to_markdown`** (`pdf.docx_to_markdown`) — (undocumented)
- **`pdf_docx_to_text`** (`pdf.docx_to_text`) — (undocumented)
- **`pdf_extract_images`** (`pdf.extract_images`) — (undocumented)
- **`pdf_fill_form`** (`pdf.fill_form`) — (undocumented)
- **`pdf_from_docx`** (`pdf.from_docx`) — (undocumented)
- **`pdf_from_html`** (`pdf.from_html`) — (undocumented)
- **`pdf_from_markdown`** (`pdf.from_markdown`) — (undocumented)
- **`pdf_from_url`** (`pdf.from_url`) — (undocumented)
- **`pdf_html_to_docx`** (`pdf.html_to_docx`) — (undocumented)
- **`pdf_images_to_pdf`** (`pdf.images_to_pdf`) — (undocumented)
- **`pdf_info`** (`pdf.info`) — (undocumented)
- **`pdf_list_form_fields`** (`pdf.list_form_fields`) — (undocumented)
- **`pdf_markdown_to_docx`** (`pdf.markdown_to_docx`) — (undocumented)
- **`pdf_merge`** (`pdf.merge`) — (undocumented)
- **`pdf_protect`** (`pdf.protect`) — (undocumented)
- **`pdf_quick`** (`pdf.quick`) — (undocumented)
- **`pdf_reorder`** (`pdf.reorder`) — (undocumented)
- **`pdf_rotate`** (`pdf.rotate`) — (undocumented)
- **`pdf_search`** (`pdf.search`) — (undocumented)
- **`pdf_set_metadata`** (`pdf.set_metadata`) — (undocumented)
- **`pdf_split`** (`pdf.split`) — (undocumented)
- **`pdf_stamp_image`** (`pdf.stamp_image`) — (undocumented)
- **`pdf_to_docx`** (`pdf.to_docx`) — (undocumented)
- **`pdf_to_html`** (`pdf.to_html`) — (undocumented)
- **`pdf_to_json`** (`pdf.to_json`) — (undocumented)
- **`pdf_to_markdown`** (`pdf.to_markdown`) — (undocumented)
- **`pdf_to_text`** (`pdf.to_text`) — (undocumented)
- **`pdf_validate`** (`pdf.validate`) — (undocumented)
- **`pdf_watermark`** (`pdf.watermark`) — (undocumented)

### phone

- **`phone_country`** (`phone.country`) — (undocumented)
- **`phone_networks`** (`phone.networks`) — (undocumented)
- **`phone_parse`** (`phone.parse`) — (undocumented)
- **`phone_parse_bulk`** (`phone.parse_bulk`) — (undocumented)
- **`phone_valid`** (`phone.valid`) — (undocumented)

### rules

- **`rules_activate`** (`rules.activate`) — (undocumented)
- **`rules_evaluate`** (`rules.evaluate`) — (undocumented)
- **`rules_publish`** (`rules.publish`) — (undocumented)
- **`rules_rollback`** (`rules.rollback`) — (undocumented)
- **`rules_service`** (`rules.service`) — (undocumented)

### secretr

- **`secretr_delete`** (`secretr.delete`) — (undocumented)
- **`secretr_get`** (`secretr.get`) — (undocumented)
- **`secretr_list`** (`secretr.list`) — (undocumented)
- **`secretr_scan`** (`secretr.scan`) — (undocumented)
- **`secretr_set`** (`secretr.set`) — (undocumented)

### securetoken

- **`securetoken_decrypt`** (`securetoken.decrypt`) — (undocumented)
- **`securetoken_encrypt`** (`securetoken.encrypt`) — (undocumented)

### server

- **`group`** (`server.group`) — (undocumented)
- **`listen`** (`server.listen`) — (undocumented)
- **`listen_async`** (`server.listen_async`) — (undocumented)
- **`middleware`** (`server.middleware`) — (undocumented)
- **`native_middleware`** (`server.native_middleware`) — (undocumented)
- **`route`** (`server.route`) — (undocumented)
- **`route_group`** (`server.route_group`) — (undocumented)
- **`server`** (`server.server`) — (undocumented)
- **`shutdown`** (`server.shutdown`) — (undocumented)
- **`static`** (`server.static`) — (undocumented)
- **`template_dir`** (`server.template_dir`) — (undocumented)
- **`web_app`** (`server.web_app`) — (undocumented)

### shamir

- **`shamir_combine`** (`shamir.combine`) — (undocumented)
- **`shamir_split`** (`shamir.split`) — (undocumented)

### tcpguard

- **`guard_middleware`** (`tcpguard.guard_middleware`) — (undocumented)
- **`tcpguard_evaluate`** (`tcpguard.evaluate`) — (undocumented)
- **`tcpguard_load`** (`tcpguard.load`) — (undocumented)
- **`tcpguard_new`** (`tcpguard.new`) — (undocumented)

### wuid

- **`wuid_new`** (`wuid.new`) — (undocumented)
- **`wuid_new_uuid`** (`wuid.new_uuid`) — (undocumented)
- **`wuid_parse`** (`wuid.parse`) — (undocumented)

### xql

- **`xql_connect`** (`xql.connect`) — (undocumented)
- **`xql_list_integrations`** (`xql.list_integrations`) — (undocumented)
- **`xql_run`** (`xql.run`) — (undocumented)

### yaml

_Also importable as: `config/yaml`_

- **`yaml_decode`** (`yaml.decode`) — yaml_decode(yamlString) decodes a YAML string into the matching value (requires config/yaml plugin)
- **`yaml_encode`** (`yaml.encode`) — yaml_encode(value[, opts]) encodes a value as a YAML string; opts supports indent (requires config/yaml plugin)

