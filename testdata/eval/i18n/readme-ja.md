<p align="center"><img src="docs/logo.svg" width="120" alt="kura"></p>

# kura — 軽量キーバリューストア

[![CI](https://img.shields.io/github/actions/workflow/status/example/kura/ci.yml)](https://github.com/example/kura/actions)
[![Go Reference](https://pkg.go.dev/badge/example.com/kura.svg)](https://pkg.go.dev/example.com/kura)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

<!-- このファイルは docs/readme.tmpl から生成されます。直接編集しないでください。 -->

## 目次

- [特徴](#特徴)
- [インストール](#インストール)
- [使い方](#使い方)
- [設定](#設定)
- [トラブルシューティング](#トラブルシューティング)
- [ライセンス](#ライセンス)

## 特徴

- 単一バイナリで動作し、外部依存はありません。
- 書き込みはログ構造化され、クラッシュ後も最後にコミットされた状態に復旧します。
- TTL 付きのキーをサポートし、期限切れのキーはバックグラウンドで削除されます。
- HTTP と gRPC の両方の API を提供します。

## インストール

```sh
go install example.com/kura/cmd/kura@latest
```

コンテナで動かす場合:

```sh
docker run -p 7400:7400 -v kura-data:/data example/kura:1.8
```

## 使い方

サーバーを起動します:

```sh
kura serve --data /var/lib/kura --listen :7400
```

値を書き込み、読み出します:

```sh
curl -X PUT localhost:7400/v1/keys/greeting -d 'こんにちは'
curl localhost:7400/v1/keys/greeting
```

Go から使う場合:

```go
c, err := kura.Dial("localhost:7400")
if err != nil {
	log.Fatal(err)
}
defer c.Close()
if err := c.Put(ctx, "greeting", []byte("こんにちは"), kura.WithTTL(time.Hour)); err != nil {
	log.Fatal(err)
}
```

## 設定

設定は `kura.yaml`、環境変数、コマンドラインフラグの順に上書きされます。

| キー | 既定値 | 説明 |
|---|---|---|
| `listen` | `:7400` | 待ち受けアドレス |
| `data` | `/var/lib/kura` | データディレクトリ |
| `sync` | `batch` | `always` は書き込みごとに fsync、`batch` は 10ms ごと |
| `max_value_bytes` | `1048576` | 値の最大サイズ |
| `compaction_interval` | `1h` | 圧縮の間隔 |

## トラブルシューティング

### 起動時に `database is locked` と表示される

別のプロセスが同じデータディレクトリを使っています。`kura status --data <dir>` で確認し、古いプロセスを停止してください。ロックファイルを手動で削除するとデータが壊れる可能性があります。

### 書き込みが遅い

`sync: always` の場合、書き込みごとにディスクへ同期するため遅くなります。耐久性の要件が許すなら `batch` を使ってください。それでも遅い場合は `kura stats` の `wal_fsync_ms` を確認してください。

### メモリ使用量が増え続ける

TTL のないキーが増え続けていないか確認してください。`kura keys --prefix <p> --count` でプレフィックスごとの件数を確認できます。

## ライセンス

MIT
