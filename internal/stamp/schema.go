package stamp

// Schema はマニフェスト(設計書 6 章)の JSON Schema。okf-stamp schema が出力する。
// エージェントは、これを読んでマニフェストを組み立てる。
const Schema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "okf-stamp manifest",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "files"],
  "properties": {
    "version": { "const": 1 },
    "mode": {
      "enum": ["fill", "overwrite", "merge"],
      "description": "全ファイルの既定のモード。省略時は --mode の値(既定 fill)"
    },
    "files": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["path", "frontmatter"],
        "properties": {
          "path": {
            "type": "string",
            "description": "カレントディレクトリからの相対パス。index.md と log.md は指定できない"
          },
          "mode": {
            "enum": ["fill", "overwrite", "merge"],
            "description": "このファイルのモード。トップレベルの mode より優先する"
          },
          "frontmatter": {
            "type": "object",
            "description": "書く frontmatter。OKF の既知のキー以外も書ける",
            "properties": {
              "type": { "type": "string", "minLength": 1, "description": "OKF で唯一の必須キー。最終的なファイルに type がなければ skip する" },
              "title": { "type": "string" },
              "description": { "type": "string" },
              "resource": { "type": "string" },
              "tags": { "type": "array", "items": { "type": "string" }, "description": "Obsidian 形式。先頭の # は除く" },
              "aliases": { "type": "array", "items": { "type": "string" } },
              "status": { "enum": ["draft", "stable", "deprecated"] },
              "stale_after": { "type": "string", "format": "date-time", "description": "ISO 8601。UTC オフセット必須" }
            },
            "additionalProperties": true
          }
        }
      }
    }
  }
}
`
