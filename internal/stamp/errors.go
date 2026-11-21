// Package stamp は concept ファイルに OKF の frontmatter を付与する。
// 入力の検証、書き換え後の内容の組み立て、マニフェストの読み込みを担う。
package stamp

// エラーコード。レポートの errors[].code に出る。機械可読にして、
// エージェントが次の質問を決める手がかりにする。
const (
	CodeMissingType       = "missing_type"
	CodeInvalidTag        = "invalid_tag"
	CodeInvalidStatus     = "invalid_status"
	CodeInvalidValue      = "invalid_value"
	CodeLegacyKey         = "legacy_key"
	CodeBrokenFrontmatter = "broken_frontmatter"
	CodeNotFound          = "not_found"
	CodeReservedName      = "reserved_name"
	CodeOutsideRoot       = "outside_root"
	CodeOutsideScope      = "outside_scope"
	CodeNotUTF8           = "not_utf8"
	CodeChangedDuringRun  = "changed_during_run"
	CodeVerifyFailed      = "verify_failed"
	CodeNotMarkdown       = "not_markdown"
	CodeUnreadable        = "unreadable"
	CodeWriteFailed       = "write_failed"
)

// Error は1つのファイルに対する入力エラー。そのファイルだけをスキップする。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errf(code, msg string) *Error { return &Error{Code: code, Message: msg} }
