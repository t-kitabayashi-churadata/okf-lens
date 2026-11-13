# okf-lens 設計ドキュメント

最終更新: 2026-11-13 / ステータス: 実装完了、公開準備中

## 1. 目的とスコープ

OKF(Open Knowledge Format)のバンドルを検索する CLI ツール。

- 対象: Markdown + YAML frontmatter で書かれた OKF の concept ファイル
- 想定規模: 1,000〜2,000 件
- 対応 OS: macOS、Windows、Linux(Ubuntu。GitHub Codespaces を含む)
- 初期版は検索専用。編集機能は作らない

OKF 仕様の出典は [SPEC.md](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)。2026-11-13 時点で v0.2 が現行版。仕様との対応は 5.1 節を参照。

## 2. 決定事項

| # | 項目 | 決定 |
|---|---|---|
| D1 | 実装言語 | Go |
| D2 | 構成 | コアライブラリ + 薄い CLI のシングルバイナリ |
| D3 | インデックス | 作らない。毎回全走査する(2,000 件規模で足りる)。遅くなったら `internal/index` を追加する |
| D4 | 必須キー | `type` のみ(OKF 準拠) |
| D5 | `type` の値 | ハードコードしない。文字列の完全一致で検索する |
| D6 | タグの階層一致 | 有効。`a/b` は `a` の検索にヒットする |
| D7 | `aliases` | 検索対象に含める |
| D8 | 任意キー検索 | 初期版から `--where key=value` を提供する |
| D9 | 大文字小文字 | すべての比較で区別しない(text、tag、type、where のキーと値) |
| D10 | 複数指定の結合 | `--tag` は AND、`--type` は OR(確定) |
| D11 | 名前 | `okf-lens`(`okf` 単体は公式と衝突するため使わない) |
| D12 | 開発環境 | Docker に閉じる。ホストに Go を要求しない。Go は 1.26(`golang.org/x/text` の要件)。コマンドは `docker compose run --rm dev ...` に統一し、`make` は使わない(Windows に標準で入っていないため) |
| D13 | `status` の省略 | 省略時は仕様どおり `stable` として扱う。`--where status=stable` は省略された文書にもヒットする |
| D14 | 並び順 | キーワード検索があるときは、一致した場所の重み付きスコアの降順。同点とキーワードなしはパスの辞書順 |
| D15 | 対応 OS | macOS、Windows、Linux(Codespaces の Ubuntu を含む)。Linux は amd64 と arm64 を配布する |
| D16 | 除外指定 | `--exclude` フラグと、検索ルートの `.okf-lensignore` の両方に対応する。書式は同じ gitignore 風の簡易版 |
| D17 | backlinks の CLI | `search --links-to <path>`(このパスにリンクしている文書)と `--linked-from <path>`(このパスがリンクしている文書)。他の条件と組み合わせられる |
| D18 | リンクの解析 | goldmark(CommonMark)を使い、`internal/okf` で抽出する |
| D19 | backlinks の対象 | OKF v0.2 に沿い、本文の Markdown リンク(§6.1)と frontmatter のパス値(§6.2)。wikilink は仕様外なので対象外 |
| D20 | 配布 | GoReleaser + GitHub Actions。macOS(arm64 / amd64)、Linux(amd64 / arm64)、Windows(amd64)。10章を参照 |
| D21 | Codespaces での開発 | 対応しない(`.devcontainer/` は作らない)。Codespaces で動かすのは、配布する Linux バイナリで足りる |

## 3. アーキテクチャ

```
cmd/okf-lens          CLI(引数解析・出力整形のみ)
   │
internal/query        検索条件の評価
   │
internal/okf          パーサーとドメインモデル(Document)
   │
internal/source       ファイル走査(fs.FS で抽象化)
   │
(将来) internal/index キャッシュ・インデックス
```

- コアと CLI を分離する。MCP サーバーや TUI を後から足せる。
- 走査は `fs.FS` に抽象化する。テストはメモリ上のファイルで書ける。
- 検索条件は `Query` 構造体にする。CLI フラグから変換する。

```go
type Query struct {
    Text  string            // title / description / aliases への部分一致
    Tags  []string          // 階層一致
    Types []string          // 完全一致
    Where map[string]string // 任意キーの一致
}
```

## 4. 検索仕様

### 4.1 条件の結合

- 異なる種類の条件(text、tag、type、where)は AND で結合する。
- 同一フラグの複数指定:
  - `--tag` は AND(すべて持つ文書)
  - `--type` は OR(いずれかに一致する文書)

### 4.2 各条件

| 条件 | 対象 | 一致方式 |
|---|---|---|
| text | `title`、`description`、`aliases` | 部分一致。大文字小文字を区別しない |
| tag | `tags` | 完全一致または階層一致。大文字小文字を区別しない。`--tag 設計` は `設計` と `設計/基本設計` にヒットする |
| type | `type` | 完全一致。大文字小文字を区別しない |
| where | 任意のトップレベルキー | スカラは完全一致。リストは要素のいずれかに一致。キー名と値の大文字小文字を区別しない |

### 4.3 スコアリング(D14)

キーワード(text)があるときだけ適用する。語ごとに、最も高い一致場所のスコアを採用し、語の合計で並べる。

| 一致した場所 | スコア |
|---|---|
| `title` と完全一致 | 100 |
| `title` に部分一致 | 60 |
| `aliases` に部分一致 | 40 |
| `description` に部分一致 | 20 |

- 空白で区切った語は AND。`認証 API` は、両方の語が title、aliases、description のどこかに一致する文書を返す。語ごとに一致場所が違ってもよい。
- 同点はパスの辞書順。結果は毎回同じ順になる。
- 本文は検索対象にしない。
- 条件が tag、type、where だけのときは、スコアを付けずパスの辞書順に並べる。

### 4.4 `where` の値の比較

| 値の型 | 比較 |
|---|---|
| 文字列 | 完全一致(大文字小文字を区別しない) |
| 数値、真偽値 | 文字列表記と比較(`priority=3`、`reviewed=true`) |
| 日付 | `YYYY-MM-DD` と比較。時刻を含む値は RFC 3339 表記と比較 |
| リスト | 要素のいずれかが一致すればよい |
| ネストしたマップ | 対象外 |
| `status` の省略 | `stable` として扱う(D13) |

同じキーを複数回指定した場合は、最後の指定が有効になる。

### 4.5 `title` の決定順

1. frontmatter の `title`
2. 本文の最初の H1
3. ファイル名(拡張子なし)

`description` は frontmatter のみを参照する。

## 5. frontmatter の扱い

Obsidian の Properties を参考にする。

| 項目 | 方針 |
|---|---|
| 形式 | 先頭の `---` で囲んだ YAML |
| `tags` | YAML リスト、またはカンマ区切り文字列を受け付ける。`#` 付きは `#` を除去する |
| `aliases` | YAML リスト、または単一文字列を受け付ける |
| 日付 | `YYYY-MM-DD` と ISO 8601 の日時を受け付ける |
| ネスト | 読み込みはするが、検索対象はトップレベルのスカラとリストに限る |
| 未知のキー | 拒否しない(OKF の規則)。`--where` で検索できる |
| 旧キー | `tag`、`alias` は扱わない |

### タグの書式(Obsidian 準拠)

- 使える文字: 英数字、日本語、`_`、`-`、`/`
- `/` は階層区切り
- 数字のみ、スペース含みは不正。警告を出して読み飛ばす

### 5.1 OKF v0.2 との対応

2026-11-13 に SPEC.md(v0.2)を確認した。

| 観点 | 仕様の内容 | okf-lens の扱い |
|---|---|---|
| 必須キー | `type` のみ | 実装済み |
| 推奨キー | `title`、`description`、`resource`、`tags` | `title`、`description`、`tags` は専用に読む。`resource` は `Fields` に保持し、`--where` で検索できる |
| ライフサイクル | `status`(`draft` / `stable` / `deprecated`。省略時は `stable`)、`stale_after`(ISO 8601、UTC オフセット付き) | `Fields` に保持し、`--where status=draft` で検索できる。省略時は `stable` として扱う(D13) |
| 来歴・信頼 | `sources`、`usage_window`、`generated`、`verified` | `Fields` に保持する。ネストした構造なので、初期版では検索対象外 |
| 追加キー | 利用者が自由に追加できる。拒否してはならない | 拒否しない |
| 予約ファイル名 | `index.md`(ディレクトリの一覧)、`log.md`(変更履歴) | concept ではないため、走査時に除外する(`internal/source` で実装) |
| v0.1 の旧形式 | `timestamp` は `generated.at` に、本文の `# Citations` は `sources` に置き換わった | 検索に使わないので、対応しない |
| バージョン宣言 | ルートの `index.md` に `okf_version: "0.2"` | 初期版では読まない |

## 6. type の語彙(サンプル)

`type` に何を書くかは、プロジェクトごとに異なる。okf-lens は語彙を定めず、制限もしない(D5)。OKF 仕様も、`type` を中央登録せず、未知の値を一般的な概念として扱うよう求めている。

以下は、ウォーターフォール開発のプロジェクトを想定したサンプル(2026-11-11 の議論、17 種)。他のプロジェクトは、自分たちの語彙を使う。

| 分類 | type |
|---|---|
| 要求・要件 | `業務要件` `機能要件` `非機能要件` `制約条件` |
| 設計対象 | `業務フロー` `画面` `帳票` `API` `バッチ` `テーブル` `外部システム連携` `アーキテクチャ` |
| 知識・判断 | `用語` `決定事項` `課題` |
| 検証 | `テストケース` `手順書` |

サンプルを使うときの運用案:

1. 工程名は `type` に入れず、`tags` かディレクトリで表す。
2. 単数形・体言止めで統一する。
3. 1 文書 1 概念にする。

ツール側の支援:

- `okf-lens types`: 走査した全文書の `type` を件数付きで一覧表示する。表記揺れ(`API` と `Web API` など)の発見に使う。
- 将来、設定ファイルで語彙を登録し、未登録の `type` に警告を出す。利用者が任意で有効にする機能とし、初期版では作らない。

## 7. CLI 仕様

```
okf-lens search [QUERY] [フラグ]
okf-lens types [フラグ]
okf-lens help
```

| フラグ | 対象 | 意味 |
|---|---|---|
| `--tag <tag>` | search | タグ。繰り返し指定すると AND。階層一致 |
| `--type <type>` | search | type。繰り返し指定すると OR |
| `--where <key=value>` | search | 任意キー。繰り返し指定すると AND。同じキーは最後の指定が有効 |
| `--exclude <pattern>` | 共通 | 除外するファイルやディレクトリ。繰り返し指定できる。7.1 節を参照 |
| `--links-to <path>` | search | このパスの文書にリンクしている文書。7.2 節を参照。繰り返し指定すると AND |
| `--linked-from <path>` | search | このパスの文書がリンクしている文書。繰り返し指定すると AND |
| `--dir <path>` | 共通 | 検索対象ディレクトリ。既定はカレント |
| `--format <table\|json>` | 共通 | 出力形式。既定は `table` |
| `--verbose` | 共通 | 読み飛ばしたファイルと警告を1件ずつ表示 |

- フラグは QUERY の前後どちらにも置ける(`search 認証 --type API`)。
- QUERY は引用符なしで複数語を渡せる(`search 認証 API`)。空白で結合して AND 検索する。
- `--tag a,b` のカンマ区切りは受け付けない。繰り返し指定する。

### 7.1 除外指定(D16)

concept ではないファイル(README.md など)を、走査の対象から外す。除外したファイルは、読み飛ばしの件数にも数えない。

指定方法は2つあり、併用すると両方を適用する。

| 方法 | 用途 |
|---|---|
| `.okf-lensignore`(検索ルート直下) | チームで共有する除外。Git に入れると、全員が同じ結果になる |
| `--exclude <pattern>` | 一時的な除外 |

```
# .okf-lensignore の例
README.md
CHANGELOG.md
drafts/
docs/**/internal.md
```

書式は gitignore の簡易版。

| 書き方 | 意味 |
|---|---|
| 空行、`#` で始まる行 | 無視する |
| `README.md` | 任意の階層の `README.md` |
| `drafts/` | 任意の階層の `drafts` ディレクトリ(末尾の `/` はディレクトリだけ) |
| `docs/draft` | 検索ルートからのパスが `docs/draft` のものだけ(`/` を含むとパス全体で照合) |
| `*`、`?` | `/` をまたがない |
| `**` | 0 個以上の階層(`docs/**/x.md`) |

- 大文字小文字を区別しない(D9)。NFC に正規化して比較する。
- `\` は `/` として扱う(Windows のパス表記を貼り付けても動く)。
- 否定(`!`)は対応しない。書くとエラーにする。
- パターンが不正なときは、`.okf-lensignore` なら行番号付きで、終了コード 2 で終わる。
- ディレクトリが除外されたら、配下は走査しない。
- `.git` や `.obsidian` など `.` で始まるディレクトリは、指定しなくても除外する。

### 7.2 backlinks 検索

「この文書にリンクしている文書」「この文書がリンクしている文書」を探す。

```
okf-lens search --links-to api/auth.md              # api/auth.md にリンクしている文書
okf-lens search --linked-from api/auth.md           # api/auth.md がリンクしている文書
okf-lens search --links-to api/auth.md --type 画面   # 他の条件と組み合わせる
```

- 複数指定は AND(`--tag` と同じ)。
- 指定するパスは、検索ルートからの相対パス。`.md` の省略、先頭の `/`、大文字小文字の違いは許容する。
- 指定した文書が存在しなくてもエラーにしない。OKF 仕様は壊れたリンクを許容する義務(MUST)を定めており、「まだ書かれていない知識」への参照を探す用途がある。存在しないときは、標準エラー出力に注意を出す。
- `--linked-from` の結果には、存在しない文書への壊れたリンクは出ない(結果は文書の一覧のため)。
- 実装: `internal/okf/links.go`(抽出)、`internal/graph`(解決と逆引き)、`internal/cli/links.go`(絞り込み)。
- 計測(2,000 文書、約 4 万リンク): 通常の検索 0.024 秒、backlinks 検索 0.050 秒。

#### 構成

```
cmd/okf-lens      フラグの解析 → graph で候補を絞る → query.Search に渡す
internal/graph    リンク先の解決と、逆引き表の構築(新規)
internal/okf      1文書ごとのリンク抽出(goldmark)
```

- `internal/query` は変更しない。
- リンクの解析は `--links-to` と `--linked-from` を使うときだけ行う。通常の検索は遅くならない。
- 1,000〜2,000 件ならインデックスなしで足りる(D3)。

#### 対象にするリンク(D19)

| 種類 | 仕様 | 扱い |
|---|---|---|
| 本文の Markdown リンク | §6.1 | 対象 |
| frontmatter のパス値 | §6.2: `resource`、`sources[].resource`、`computation`、`executor.resource`、`attester.resource` | 対象。バンドル内の `.md` に解決できた値だけをリンクとして扱う |
| wikilink `[[名前]]` | 仕様外 | 対象外。将来の拡張として残す |

#### リンク先の解決規則

| 対象 | 扱い |
|---|---|
| `/a/b.md` | バンドルルートからのパス |
| `./b.md`、`../c.md` | リンク元ファイルのディレクトリ基準で解決し、`a/b.md` の形に正規化(frontmatter のパスも同じ) |
| `#見出し`、`?query` | 除去する。`#見出し` だけのリンクは自己参照なので無視 |
| `https://`、`mailto:` など | 外部リンクなので無視 |
| `%E8%AA%8D...` | パーセントエンコードを復号する |
| 画像、`.md` 以外 | 無視 |
| コードブロック内 | 無視(goldmark が判定する) |
| 自己リンク、同じ先への重複 | 1つにまとめる |
| 大文字小文字 | 区別しない(D9) |
| 壊れたリンク | エラーにしない |
| `index.md`、`log.md` | リンク元として数えない(全 concept にリンクするため、ノイズになる) |
| 除外指定(D16)で外した文書 | グラフに含めない |

### 出力

標準出力に結果だけを書き、件数や警告は標準エラー出力に書く。パイプで次のコマンドに渡しても結果が混ざらない。

`search` の table 形式は、列が `TYPE`、`TITLE`、`PATH` の順。全角文字を2桁として数え、列を揃える。

```
TYPE  TITLE         PATH
API   認証API       api/auth.md
画面  ログイン画面  screen/login.md
```

`search` の json 形式は、`path`、`type`、`title`、`description`、`tags`、`aliases`、`score` を持つオブジェクトの配列。0 件のときは `[]`。

`types` は、type の表記ごとに件数を数える。検索は大文字小文字を区別しないが、`API` と `api` のような表記揺れを見つけるため、表記ごとに別の行にする。

### 標準エラー出力

| 内容 | 表示 |
|---|---|
| 検索結果の件数 | `N 件`(`search` のみ) |
| 読み飛ばしたファイル、警告 | 件数だけ。`--verbose` で1件ずつ表示 |

frontmatter や `type` がないファイル(README など)は、読み飛ばしとして数える。数えたくないファイルは、7.1 節の除外指定で消せる。

### 終了コード

| コード | 意味 |
|---|---|
| 0 | 正常終了(ヒット 0 件を含む) |
| 1 | 実行エラー(ディレクトリが存在しない、出力に失敗など) |
| 2 | 引数エラー(不明なフラグ、不正な `--format`、`--where` の書式違いなど) |

ヒット 0 件を 0 にする理由は、`grep` と違い、スクリプトでエラー扱いにされる事故を避けるため。

## 8. macOS / Windows / Linux の差異対策

| 項目 | 対策 |
|---|---|
| パス区切り | `filepath` と `fs.FS` を使う。文字列連結しない |
| 改行 | CRLF と UTF-8 BOM を許容する |
| Unicode 正規化 | macOS のファイル名は NFD になりやすい。クエリ、frontmatter、本文を NFC に統一してから比較する |
| 大文字小文字 | すべての比較で区別しない(D9)。NFC 正規化後に Go の `strings.EqualFold` 相当で比較する |
| 出力 | UTF-8 固定。Windows コンソールの文字化けを CI で確認する |
| シェル | `bash` 前提の処理を書かない |
| ファイルシステム | Linux は大文字小文字を区別する。`Auth.md` と `auth.md` が共存しうる。検索の比較は区別しないが、パスは別ファイルとして扱う |
| Linux の動作確認 | 開発用コンテナが Linux(Debian)のため、`go test` は常に Linux 上で実行される。Codespaces の Ubuntu とは同じ Linux カーネルの系統で、Go のシングルバイナリは `CGO_ENABLED=0` で依存なく動く |

## 9. 開発環境

- `Dockerfile`: `golang:1.26-bookworm` ベースの開発イメージ。`CGO_ENABLED=0` 固定
- `compose.yaml`: `dev` サービス。ソースをバインドマウントし、モジュールとビルドのキャッシュは名前付きボリュームに置く
- 実行例は [README](../README.md) の「開発環境」を参照
- CI でも同じイメージを使い、手元と CI の差をなくす

## 10. 配布とCI

### 配布物(D20)

GoReleaser が、タグ(`v*`)から次のファイルを GitHub Releases に公開する。

| OS | CPU | 形式 |
|---|---|---|
| macOS | arm64、amd64 | `.tar.gz` |
| Linux(Codespaces を含む) | amd64、arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

- ファイル名は `okf-lens_<version>_<os>_<arch>`。`checksums.txt` を付ける。
- Windows の arm64 は配布しない。amd64 のエミュレーションで動く。
- `CGO_ENABLED=0`、`-trimpath`、`-s -w` でビルドする。ランタイムも外部ライブラリも不要。
- バージョンは `-ldflags` で `internal/cli.Version` に埋め込む。`okf-lens version` で確認できる。開発ビルドは `dev`。
- 設定は `.goreleaser.yaml`。

### CI(`.github/workflows/ci.yml`)

- `push`(main)と `pull_request` で、macOS、Windows、Ubuntu の3つでテストする。
- 手元の開発は Docker に閉じるが、CI は各 OS のネイティブ環境で動かす。パス区切り、改行、ファイルシステムの実際の差を検出するため。
- `gofmt` の確認は Ubuntu だけで行う。`go vet` と `go test` は3つすべてで行う。
- `.gitattributes` で改行を LF に固定し、Windows の checkout で `gofmt` や期待値が崩れるのを防ぐ。

### リリース(`.github/workflows/release.yml`)

1. `v*` のタグを push する。
2. `go test` を通す。
3. GoReleaser が、ビルド、アーカイブ、チェックサム作成、GitHub Releases への公開を行う。

### 手元でのドライラン

```sh
docker compose run --rm goreleaser
```

公開はせず、`dist/` にバイナリを出力する。Git リポジトリで、コミットが1つ以上ある状態が必要。

### 配布先(今回は対象外)

Homebrew tap、Scoop、winget への登録は、リリースの運用が安定してから検討する。GoReleaser の設定に追記すれば自動化できる。

## 11. 未決事項

- [ ] **backlinks の出力**: 一致した文書のリンク元の行番号や前後の文章を出すか(初期版では出さない案)
- [ ] GitHub のリポジトリ作成と、名前の空き確認(GitHub org、Homebrew)。リリースの公開に必要
- [ ] Windows 実機での動作確認(保留中。CI の windows-latest でテストは通る想定だが、未実行)

## 12. 次の一手

1. ~~`go mod init` と `internal/okf` のモデル定義~~(完了)
2. ~~frontmatter パーサーとテスト(NFC、CRLF、BOM、タグ書式)~~(完了)
3. ~~`internal/source`: ディレクトリ走査(`index.md` と `log.md` は除外)~~(完了)
4. ~~`internal/query` の実装~~(完了)
5. ~~CLI の `search` と `types`~~(完了)
6. ~~除外指定(`--exclude` と `.okf-lensignore`)~~(完了)
7. ~~backlinks の実装(`internal/okf` のリンク抽出、`internal/graph`、CLI のフラグ)~~(完了)
8. ~~GoReleaser と GitHub Actions(macOS、Windows、Ubuntu の matrix)~~(設定は完了。公開には GitHub リポジトリが必要)
9. Windows 実機での確認(標準出力の文字化け、パス区切り)。保留中
