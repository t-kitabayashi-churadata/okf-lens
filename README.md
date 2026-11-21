# okf-lens

OKF(Open Knowledge Format)のバンドルを検索する CLI ツール。macOS、Windows、Linux(GitHub Codespaces の Ubuntu を含む)で動く。

> ステータス: `search`(backlinks を含む)と `types` を実装済み。`okf-stamp`(frontmatter の付与)も同梱。配布の設定(GoReleaser、GitHub Actions)まで完了。詳細は [docs/design.md](docs/design.md)。

## できること

- `title`、`description`、`aliases` への部分一致検索
- `tags` による絞り込み(階層一致: `設計` で `設計/基本設計` もヒット)
- `type` による絞り込み
- 任意の frontmatter キーによる絞り込み(`--where`)
- リンクによる絞り込み(`--links-to` で backlinks、`--linked-from` でリンク先)

## 使い方

```sh
# キーワード検索
okf-lens search "認証"

# タグと type の組み合わせ
okf-lens search --tag 基本設計 --type API

# 任意キーで絞り込み
okf-lens search --where status=draft

# JSON で出力
okf-lens search "認証" --format json

# 使われている type を件数付きで一覧表示
okf-lens types

# backlinks: この文書にリンクしている文書
okf-lens search --links-to api/auth.md

# この文書がリンクしている文書(type と組み合わせ)
okf-lens search --linked-from screen/login.md --type 用語

# README.md や下書きを除外
okf-lens search "認証" --exclude README.md --exclude drafts/
```

出力例(`search` の table 形式。件数と警告は標準エラー出力):

```
TYPE  TITLE         PATH
API   認証API       api/auth.md
画面  ログイン画面  screen/login.md
```

条件の結合ルール:

- 異なる種類の条件は AND
- `--tag` の複数指定は AND
- `--type` の複数指定は OR
- 空白で区切ったキーワードは AND(`search 認証 API`)
- すべての比較で大文字小文字を区別しない
- 並び順はキーワードがあるとき、title、aliases、description の順に一致を重く見る

## リンク検索

OKF v0.2 のリンクを対象にする。

- 本文の Markdown リンク(`[text](/a/b.md)`、`[text](./b.md)`)
- frontmatter のパス値(`resource`、`sources[].resource`、`computation`、`executor.resource`、`attester.resource`)

`--links-to` と `--linked-from` に渡すパスは、検索ルートからの相対パス。`.md` と先頭の `/` は省略できる。存在しない文書も指定できる(壊れたリンクの参照元を探せる)。`index.md` と `log.md` からのリンクは数えない。

## 除外ファイル

README.md など concept ではないファイルは、検索ルートに `.okf-lensignore` を置いて除外する。書式は gitignore の簡易版。Git に入れると、チーム全員が同じ結果になる。

```
# .okf-lensignore
README.md
drafts/
docs/**/internal.md
```

一時的な除外は `--exclude` で指定する。詳細は [設計ドキュメント](docs/design.md) の 7.1 節。

## 対象ファイルの例

```markdown
---
type: API
title: 認証API
description: ログインとトークン発行
tags: [基本設計/認証, api]
aliases: [ログインAPI]
---

本文
```

必須キーは `type` のみ。それ以外のキーは自由に使える。

## okf-stamp: frontmatter を付与する

同じリポジトリの別のバイナリ。`.md` ファイルに OKF の frontmatter を書く。既存の本文と書式は変えない。既定は dry-run で、`--write` を付けたときだけ書く。設計は [docs/design-okf-stamp.md](docs/design-okf-stamp.md)。

```sh
# 状態を調べる(変更しない)
okf-stamp scan docs

# docs 配下に type とタグを付ける(dry-run)
okf-stamp apply docs --type API --tag 設計

# 反映する
okf-stamp apply docs --type API --tag 設計 --write

# ファイルごとの値は JSON のマニフェストで渡す(形式は okf-stamp schema)
okf-stamp apply --manifest manifest.json --write

# 大量のファイルを、重ならないグループに分ける
okf-stamp partition docs --max-files 100
```

- 対象は PATH(ファイルまたはフォルダ)で指定する。実行時のカレントディレクトリの外には出られない。
- 既存の値の扱いは `--mode`(`fill`、`overwrite`、`merge`)で決める。
- 結果は表か `--format json`。JSON の `changes` に、キーごとの変更前と変更後が入る。
- 終了コードは 0(成功)、1(実行エラー)、2(引数エラー)、3(一部をスキップ)。

## 設計方針

- Go のシングルバイナリ。ランタイム不要
- コアライブラリと CLI を分離する
- インデックスは持たず、毎回全走査する(1,000〜2,000 件を想定)

## 開発環境

開発に必要なのは **Docker だけ**。Go のインストールは不要。コマンドは macOS、Windows(PowerShell)で共通。

```sh
# 初回: 開発イメージをビルド
docker compose build

# テスト
docker compose run --rm dev go test ./...

# 静的解析とフォーマット確認
docker compose run --rm dev go vet ./...
docker compose run --rm dev gofmt -l .

# 依存の整理
docker compose run --rm dev go mod tidy

# 実行(コンテナ内。検索対象はマウントしたプロジェクト配下)
docker compose run --rm dev go run ./cmd/okf-lens search "認証" --dir ./docs

# 配布用バイナリのビルド(bin/ に出力)
docker compose run --rm -e GOOS=linux   -e GOARCH=amd64 dev go build -o bin/okf-lens-linux-amd64 ./cmd/okf-lens
docker compose run --rm -e GOOS=darwin  -e GOARCH=arm64 dev go build -o bin/okf-lens-darwin-arm64 ./cmd/okf-lens
docker compose run --rm -e GOOS=windows -e GOARCH=amd64 dev go build -o bin/okf-lens-windows-amd64.exe ./cmd/okf-lens

# コンテナ内のシェル
docker compose run --rm dev bash
```

Go のバージョンは `Dockerfile` で固定している。モジュールキャッシュとビルドキャッシュは Docker の名前付きボリュームに残るため、2回目以降は速い。

## インストール

[GitHub Releases](../../releases) から、OS に合うファイルをダウンロードして展開する。

| OS | ファイル |
|---|---|
| macOS(Apple Silicon) | `okf-lens_<version>_darwin_arm64.tar.gz` |
| macOS(Intel) | `okf-lens_<version>_darwin_amd64.tar.gz` |
| Linux、GitHub Codespaces | `okf-lens_<version>_linux_amd64.tar.gz` |
| Linux(arm64) | `okf-lens_<version>_linux_arm64.tar.gz` |
| Windows | `okf-lens_<version>_windows_amd64.zip` |

展開した `okf-lens`(Windows は `okf-lens.exe`)を、PATH の通ったディレクトリに置く。`okf-lens version` で確認できる。

## リリース手順(メンテナ向け)

```sh
# 手元でのドライラン(公開しない。dist/ にバイナリを出力)
docker compose run --rm goreleaser

# 公開: タグを push すると GitHub Actions が GitHub Releases に公開する
git tag v0.1.0
git push origin v0.1.0
```

## ドキュメント

- [設計ドキュメント](docs/design.md): 決定事項、検索仕様、frontmatter の扱い、未決事項

## ライセンス

未定
