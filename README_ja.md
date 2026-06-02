[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct) [![LLM Friendly](https://img.shields.io/badge/LLM-Friendly-blue)](llms.txt)

> [!NOTE]
> **AI Agents**: AIエージェント開発用のリファレンスおよび実装ルールは [llms-full.txt](llms-full.txt) を参照してください。

# binarystruct: GO構造体のバイナリ変換

「binarystruct」は生のバイナリデータとGo構造体の間の変換を半自動かするためのパッケージです。

普段Goでバイナリデータを扱う場合は謹製の「encoding/binary」を使うことが多いです。こちらは使いやつくはありますが、構造体側のデータ長と生のデータ長が一致しなければならず、変換した値を実際にコード側で使う場合に型変換が必要が場合があります。
例えば、１バイトないし２バイトで記録された数字を使う前にint()への変換が必要な場合がよくあります。

このパッケージはそのような型変換を構造体のタグを参考することにより（半）自動化するためのものです。


## 実例

例えば以下のようなバイナリ生データがあるとします。4バイトの識別子とそれぞれ１バイト・２バイト・４バイトの数字です。
これをGo側では文字列と３つのint型として読み込みたい場合、構造体の定義で以下のようなタグ付けを行えば、読み込み時に自動で変換が行われます。もちろん逆もOKです。

```
// 生データ
blob := []byte { 0x61, 0x62, 0x63, 0x64,
	0x01,
	0x00, 0x02,
	0x00, 0x00, 0x00, 0x03 }
// [ "abcd", 0x01, 0x0002, 0x00000003 ]

// フィールドタグ付きの構造体
strc := struct {
	Header       string `binary:"[4]byte"` // maps to 4 bytes
	ValueInt8    int    `binary:"int8"`    // maps to single signed byte
	ValueUint16  int    `binary:"uint16"`  // maps to two bytes
	ValueDword32 int    `binary:"dword"`   // maps to four bytes
}{}

// バイナリ→構造体変換
readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

// 出力テスト
fmt.Println(strc)
// {abcd 1 2 3}

// 構造体→バイナリ
output, err := binarystruct.Marshal(&strc, binarystruct.BigEndian)
// output == blob

```

## 主な機能

* **自動的かつ安全な型変換**: パックされたバイナリレイアウトをGoの自然な型（例: `uint16` や `int8` ストリームから直接Goの `int` フィールド）へ、安全な値の範囲チェック付きで自動変換します。
* **静的コード生成**: `binarystruct-codegen` ツールにより、構造体タグからリフレクション不要で最適化された `MarshalBinary` / `UnmarshalBinary` メソッドを生成します。Safeモードのリフレクションに対して最大**6.7倍の高速化**をほぼゼロアロケーションで実現し、`go:generate` との統合をサポートします。詳細は [`binarystruct-codegen/README.md`](binarystruct-codegen/README.md) を参照してください。
* **超高速ランタイムインタプリタ**: タグ解析結果を初回ロード時にキャッシュし、Unsafeモード（デフォルト）では `unsafe.Pointer` とアロケーションフリーのスライス転送技術を用いることで、標準Goリフレクションと比較して最大**214倍の高速化**と**99.9%のメモリ割り当て削減**を実現します。
* **宣言的バリデーション**: `range=min..max` による数値範囲チェックや `match=pattern` による正規表現文字列バリデーションにより、デシリアライズ時にインラインで検証を行い、違反時にはエラーを返します。
* **きめ細かなレイアウト制御**: `byte`, `word`, `dword`, `qword` などの明示的なデータ型や、`pad(size)` によるゼロ埋めパディングを柔軟に設定できます。
* **動的なサイズ計算式**: 配列の長さや文字列バッファサイズを、四則演算（`+`, `-`, `*`, `/`）と括弧 `()` を用いて他の構造体フィールドの値から動的に決定できます（例: `[PayloadSize - (HeaderLength * 2)]byte`）。
* **ポリモーフィズムとインターフェース処理**: 事前に割り当てられたインターフェースへの直感的な復元、またはカスタムシリアライザにより、デコード済みのヘッダー情報を基にした実行時の動的型割り当てに対応します。
* **多言語テキストエンコーディング**: `AddTextEncoding` で文字コード（例: `Shift-JIS`, `UTF-16`）をあらかじめ登録しておくことで、文字列フィールドに対して文字コード変換に対応し、フォールバック用のデフォルトエンコードを設定できます。
* **フィールド単位のエンディアン制御**: フィールドごとにエンディアン（`big`, `little`, `inverse`（反転））を指定でき、ネストされた構造体へも再帰的に伝播します。
* **単一値のシリアライズ**: 構造体でない変数単体に対しても、カスタムタグを指定して `MarshalAs` / `UnmarshalAs` で直接エンコード/デコードできます。
* **カスタムシリアライザ**: `Serializer` インターフェースを実装して Marshaller に登録することで、複雑なデータ検証や動的マッピングを処理できます。
* **構造体レイアウト検証ヘルパー**: 構造体のメモリ上のオフセット、サイズ、型、値を10進数/16進数/2進数でカスタマイズして可視化できる `Inspect` API を提供します。
* **Safeモードへのフォールバック**: Google App Engineなどでの実行環境制限がある場合、`-tags safe_binarystruct` ビルドフラグで純粋なリフレクションによる標準Go実装に切り替え可能です。

## 動作モード（Safe vs. Unsafe / SIMD）

パフォーマンス要件や実行環境の制約、実験的なハードウェア支援に合わせた複数のビルドモードをサポートしています。

| モード / ビルドタグ | 概要 | パフォーマンス・特徴 |
| :--- | :--- | :--- |
| **デフォルト（Unsafe）** | `unsafe.Pointer` インタプリタとレイアウト適合スライスの高速処理パスを用いて、リフレクションなしで直接メモリアドレスにアクセスします。 | **最高速度**（最大214倍高速、メモリ割り当てを99.9%削減）。 |
| **Safeモード** (`-tags safe_binarystruct`) | 純粋なリフレクションのみを用いる標準Go実装にフォールバックします。Google App Engineなどのセキュリティ上の制限がある環境で必須。 | リフレクションによる標準的なオーバーヘッド。 |
| **SIMDモード** (`GOEXPERIMENT=simd -tags experiment_simd`) | Go 1.26 の実験的パッケージ `simd/archsimd` を用いて、AMD64上でのエンディアン変換（バイトスワップ）をベクター命令で処理します（CPU機能検知付き）。 | 大きな数値配列やスライスのベクター化によるスループット最大化。 |

### 制限されたプラットフォーム向けのビルド

メモリアドレスへのアクセス制限や、Goの `unsafe` パッケージの使用が禁止されているサンドボックス環境（例：Google App Engine 標準環境）へデプロイする場合は、`safe_binarystruct` ビルドタグを有効にしてプロジェクトをコンパイルする必要があります：

```bash
go build -tags safe_binarystruct ./...
go test -tags safe_binarystruct ./...
```

---

## 構造体レイアウトの可視化とデバッグ

`binarystruct` には、構造体のバイナリレイアウトを解析し、各フィールドのオフセット、サイズ、値を表示する `Inspect` ヘルパーが含まれています。これは、バイトアライメントやパディングの問題をデバッグする際に非常に便利です。

```go
type Packet struct {
	Magic  string `binary:"[4]byte"`
	Length uint16 `binary:"uint16"`
	Flag   uint8  `binary:"uint8"`
	Data   []byte `binary:"[2]byte"`
}

pkt := Packet{Magic: "HEAD", Length: 12, Flag: 1, Data: []byte{0xaa, 0xbb}}

// 構造体のレイアウトを解析
layout, _ := binarystruct.Inspect(&pkt, binarystruct.BigEndian)

// フォーマットを設定して表示
format := binarystruct.DefaultLayoutFormat
format.BaseOffset = 16 // オフセットを16進数でフォーマット
fmt.Println(layout.Format(format))
```

出力結果:
```text
+0x00(0x04) [4]byte Magic = [72 69 65 68] ("HEAD")
+0x04(0x02) uint16 Length = 12 (0x000c)
+0x06(0x01) uint8 Flag = 1 (0x01)
+0x07(0x02) [2]byte Data = [170 187]
```

> **注意**: 構造体にカスタムシリアライザやエンコーディングを使用している場合は、パッケージレベルの `binarystruct.Inspect(&pkt, ...)` ではなく、設定済みの Marshaller インスタンスの `marshaller.Inspect(&pkt, ...)` を使用してください。これにより、検証時にカスタム設定が正しく認識されます。

### レイアウトのJSON出力

解析されたレイアウトのメタデータをJSONスキーマとして出力することができます。これは、外部システムとの統合や他言語でのスキーマ構造の自動生成に便利です：

```go
js, _ := layout.ToJSON()
fmt.Println(string(js))
```

---

## プロダクション向けの静的コード生成（Codegen）

`binarystruct` は、構造体のレイアウト定義から静的なGoメソッドを自動生成するスタンドアロンのコードジェネレータツールを提供しています。プロダクション環境で使用することで、実行時のレイアウト解析やリフレクションのオーバーヘッドを完全に排除し、最大のパフォーマンスを得ることができます。

### インストール
コードジェネレータのCLIツールをインストールします：
```bash
go install github.com/mixcode/binarystruct/binarystruct-codegen@latest
```

### 使用方法
対象となる構造体の静的な `MarshalBinary` および `UnmarshalBinary` メソッドを生成します：
```bash
binarystruct-codegen -type MyStruct,MyNestedStruct [対象のパッケージディレクトリ]
```
デフォルトでは、指定した最初の型名を元にした `<型名>_binary.go` が同ディレクトリに出力されます。

### go generate との連携
通常、Goソースファイル内に `go generate` コメントを追加して連携することをお勧めします：
```go
//go:generate binarystruct-codegen -type Packet,Header
type Packet struct {
	Magic uint32 `binary:"uint32"`
	Data  []byte `binary:"[10]byte"`
}
```
`go generate ./...` を実行することで、シリアライズメソッドが自動生成・コンパイルされます。

### 仕組みと特徴
* 生成されたコードは、Go標準の `encoding.BinaryMarshaler` および `encoding.BinaryUnmarshaler` インタフェースの他、高パフォーマンスなストリーミングインタフェース（`BinaryReader` / `BinaryWriter`）を実装します。
* カスタムシリアライザやテキストエンコーディングが指定されている場合、コンテキスト対応インタフェース（`MarshallerContextReader` / `MarshallerContextWriter`）が生成され、実行時に自動的に [Marshaller] コンテキストから適切なハンドラを取得します。
* パッケージレベルの `binarystruct.Marshal` / `binarystruct.Unmarshal` を使用している場合でも、対象オブジェクトが生成されたメソッドを実装している場合は、自動的にリフレクションをバイパスして生成されたメソッドを高速実行（ファストパス）します。

### 性能比較

バリデーションルール（範囲チェック）、8つのフィールドを持つネストした構造体、動的なバイトスライスを含む実用的な280バイトのパケットのデシリアライズ処理について、各モードごとの性能比較ベンチマーク結果は以下の通りです（13th Gen Intel Core i5-13600Kにて測定）：

| 実行モード / 処理戦略 | 実行時間 | ヒープアロケーション（メモリ割当） | 性能向上率 |
| :--- | :--- | :--- | :--- |
| **Safe Mode** (`-tags safe_binarystruct`) | `4,260 ns/op` | `47 allocs/op` | 基準値（Baseline） |
| **Unsafe Mode** (デフォルト・インタプリタ) | `3,670 ns/op` | `22 allocs/op` | 速度+16%、メモリ割当-53% |
| **静的コード生成** (Codegen適用・コンパイル済) | `634 ns/op` | `8 allocs/op` | **速度+570%（約6.7倍高速）**、**メモリ割当-83%** |

---

## バイトオフセット付きの詳細なエラーレポート

バイナリデータのデシリアライズ（Unmarshal）中にエラー（予期しないEOFなど）が発生した場合、エラーはカスタム構造体 `DecodeError` にラップされて返されます。これにより、失敗が発生した正確なバイトオフセットとフィールド名を特定できます：

```go
_, err := binarystruct.Unmarshal(corruptedData, binarystruct.BigEndian, &pkt)
if err != nil {
	var decodeErr *binarystruct.DecodeError
	if errors.As(err, &decodeErr) {
		fmt.Printf("エラー発生位置 (バイトオフセット): %d, フィールド: %q, 詳細: %v\n", 
			decodeErr.Offset, decodeErr.Field, decodeErr.Err)
	}
}
```

---

## 関連情報・ドキュメント
* [構造体タグ リファレンスマニュアル](STRUCT_TAGS_ja.md) - タグの対応型一覧、パラメータ一覧、動的計算式について。
* [Goドキュメント](https://pkg.go.dev/github.com/mixcode/binarystruct) - API仕様について。

