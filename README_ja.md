[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct) [![LLM Friendly](https://img.shields.io/badge/LLM-Friendly-blue)](llms.txt)

> [!NOTE]
> **AI Agents**: AIエージェント開発用のリファレンスおよび実装ルールは [llms-full.txt](llms-full.txt) を参照してください。

> [!IMPORTANT]
> **0.2.x からの移行:** v0.3.0 には破壊的変更があります（`Marshaler`/`Codec` への改名、構造体でのバイトオーダー宣言、バイトオーダー引数のない API）。**[MIGRATION.md](MIGRATION.md)** を参照してください。

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

// フィールドタグ付きの構造体。先頭の空フィールド `_` がこの構造体のバイトオーダーを
// 宣言するため、Marshal/Unmarshal にバイトオーダー引数は不要です。
strc := struct {
	_            struct{} `binary:"endian=big"` // この構造体はビッグエンディアン
	Header       string   `binary:"[4]byte"`    // maps to 4 bytes
	ValueInt8    int      `binary:"int8"`        // maps to single signed byte
	ValueUint16  int      `binary:"uint16"`      // maps to two bytes
	ValueDword32 int      `binary:"dword"`       // maps to four bytes
}{}

// バイナリ→構造体変換
readsz, err := binarystruct.Unmarshal(blob, &strc)

// 出力テスト
fmt.Println(strc)
// {{} abcd 1 2 3}

// 構造体→バイナリ
output, err := binarystruct.Marshal(&strc)
// output == blob

```

> **タグ.** `binary:"…"` タグは各フィールドのバイナリ表現（ワイヤフォーマット）を宣言します。固定幅のスカラー（`int8`〜`int64`、`uint8`〜`uint64`、`float32`/`float64`、および `byte`/`word`/`dword`/`qword` のエイリアス）、配列、文字列バリアント（長さプレフィックス付き・ゼロ終端）がサポートされています。詳細は[構造体タグリファレンス](STRUCT_TAGS_ja.md#2-バイナリ型対応型一覧)を参照してください。

> **バイトオーダー**: 構造体のバイトオーダーは、空フィールド `_ struct{}` に `binary:"endian=big|little"` タグを付けて一度だけ宣言します（またはそれを宣言した構造体を埋め込みます）。すると `Marshal`/`Unmarshal`/`Write`/`Read`/`Append`/`Inspect` はバイトオーダー引数を取りません。フィールド単位の `endian=` タグはそのフィールドのみを上書きします。バイトオーダーを宣言しない値（裸のスカラーや外部の構造体）には `binarystruct.NewMarshalerOrder(order)` でフォールバックを指定してください。

## 主な機能

* **自動的かつ安全な型変換**: パックされたバイナリレイアウトをGoの自然な型（例: `uint16` や `int8` ストリームから直接Goの `int` フィールド）へ、安全な値の範囲チェック付きで自動変換します。
* **静的コード生成**: `binarystruct-codegen` ツールにより、構造体タグからリフレクション不要で最適化された `MarshalBinary` / `UnmarshalBinary` メソッドを生成します。リフレクションインタプリタに対して**数倍の高速化**を全モード中最小のアロケーションで実現し、`go:generate` との統合をサポートします。詳細は [`binarystruct-codegen/README.md`](binarystruct-codegen/README.md) を参照してください。
* **超高速ランタイムインタプリタ**: タグ解析結果を初回ロード時にキャッシュし、Unsafeモード（デフォルト）では `unsafe.Pointer` とアロケーションフリーのスライス転送技術を用いることで、Safeモード（リフレクション）と比較して高速化とメモリ割り当て削減を実現します（具体的な数値は[性能比較](#性能比較)を参照）。
* **宣言的バリデーション**: `range=min..max` による数値範囲チェックや `match=pattern` による正規表現文字列バリデーションにより、デシリアライズ時にインラインで検証を行い、違反時にはエラーを返します。
* **きめ細かなレイアウト制御**: `byte`, `word`, `dword`, `qword` などの明示的なデータ型や、`pad(size)` によるゼロ埋めパディングを柔軟に設定できます。
* **動的なサイズ計算式**: 配列の長さや文字列バッファサイズを、四則演算（`+`, `-`, `*`, `/`）と括弧 `()` を用いて他の構造体フィールドの値から動的に決定できます（例: `[PayloadSize - (HeaderLength * 2)]byte`）。
* **多次元配列**: 角括弧を重ねる（`[2][3]int16`、`[Rows][Cols]uint8`）ことで、ネストした Go の配列・スライスを行優先（row-major）順でエンコード／デコードできます。各次元はそれぞれ独立した計算式です。詳細は[構造体タグリファレンス](STRUCT_TAGS_ja.md#4-配列およびバッファサイズ表記)を参照してください。
* **計算・派生フィールド**: `valueof=bytelen(F)` / `valueof=count(F)` を使うと、エンコード時に長さや要素数のフィールドを自動的に埋められます。`len(Name)` と一致させ続けなければならない `NameLen` を手動管理する必要がなくなります。CRC・チェックサムなどの派生値には、`Marshaler.AddValueOf` で**カスタム評価関数**を登録し `valueof=CRC32(Type, Data)` のように参照します（エンコード時に計算、デコード時に検証）。詳細は[計算フィールド値](#計算フィールド値valueof)を参照してください。
* **固定値・マジックナンバー**: `const=` でシグネチャやバージョンフィールドを固定できます。エンコード時に書き込み、デコード時に検証します（整数マジック `const=0x04034b50` やバイト列マジック `const=0x89504e470d0a1a0a`）。詳細は[固定値・マジックナンバー](#固定値マジックナンバーconst)を参照してください。
* **ポリモーフィズムとインターフェース処理**: 事前に割り当てられたインターフェースへの直感的な復元、またはカスタムコーデックにより、デコード済みのヘッダー情報を基にした実行時の動的型割り当てに対応します。
* **多言語テキストエンコーディング**: `AddTextEncoding` で文字コード（例: `Shift-JIS`, `UTF-16`）をあらかじめ登録しておくことで、文字列フィールドに対して文字コード変換に対応し、フォールバック用のデフォルトエンコードを設定できます。
* **フィールド単位のエンディアン制御**: フィールドごとにエンディアン（`big`, `little`, `inverse`（反転））を指定でき、ネストされた構造体へも再帰的に伝播します。
* **単一値のシリアライズ**: 構造体でない変数単体に対しても、カスタムタグを指定して `MarshalAs` / `UnmarshalAs` で直接エンコード/デコードできます。
* **カスタムコーデック**: `Codec` インターフェースを実装して Marshaler に登録することで、複雑なデータ検証や動的マッピングを処理できます。
* **構造体レイアウト検証ヘルパー**: 構造体のメモリ上のオフセット、サイズ、型、値を10進数/16進数/2進数でカスタマイズして可視化できる `Inspect` API を提供します。
* **Safeモードへのフォールバック**: Google App Engineなどでの実行環境制限がある場合、`-tags safe_binarystruct` ビルドフラグで純粋なリフレクションによる標準Go実装に切り替え可能です。

## 動作モード（Safe vs. Unsafe / SIMD）

パフォーマンス要件や実行環境の制約、実験的なハードウェア支援に合わせた複数のビルドモードをサポートしています。

| モード / ビルドタグ | 概要 | パフォーマンス・特徴 |
| :--- | :--- | :--- |
| **デフォルト（Unsafe）** | `unsafe.Pointer` インタプリタとレイアウト適合スライスの高速処理パスを用いて、リフレクションなしで直接メモリアドレスにアクセスします。 | Safeモードより高速でアロケーションも少ない（[性能比較](#性能比較)を参照）。 |
| **Safeモード** (`-tags safe_binarystruct`) | 純粋なリフレクションのみを用いる標準Go実装にフォールバックします。Google App Engineなどのセキュリティ上の制限がある環境で必須。 | リフレクションによる標準的なオーバーヘッド。 |
| **SIMDモード** (`GOEXPERIMENT=simd -tags experiment_simd`) | Go 1.26 の実験的パッケージ `simd/archsimd` を用いて、AMD64上でのエンディアン変換（バイトスワップ）をベクター命令で処理します（CPU機能検知付き）。 | 大きな数値配列やスライスのベクター化によるスループット最大化。 |

### 制限されたプラットフォーム向けのビルド

メモリアドレスへのアクセス制限や、Goの `unsafe` パッケージの使用が禁止されているサンドボックス環境（例：Google App Engine 標準環境）へデプロイする場合は、`safe_binarystruct` ビルドタグを有効にしてプロジェクトをコンパイルする必要があります：

```bash
go build -tags safe_binarystruct ./...
go test -tags safe_binarystruct ./...
```

---

## 計算フィールド値（`valueof`）

長さや要素数のフィールドは、通常は別のフィールドに手作業で合わせ続ける必要があります。`valueof` オプションはこうしたフィールドのシリアライズ値を**エンコード時に**計算するため、データ側のフィールドだけを管理すれば済みます。組み込みは `bytelen(F)`（任意フィールドのエンコード後バイト長）と `count(F)`（配列・スライスの要素数）。CRC などの派生値には `Marshaler.AddValueOf` で**カスタム評価関数**（例: `valueof=CRC32(Type, Data)`）を登録でき、デコード時に検証も行われます。`valueof` は **emit-only** で、ストリームには書き込みますが Go のフィールドは変更しません（値を取り込むには `Unmarshal` でラウンドトリップ）。詳細は[構造体タグリファレンス](STRUCT_TAGS_ja.md#8-計算フィールド値valueof)を参照してください。

### レシピ: 可変長レコード

実際のバイナリ形式で最もよく現れるレイアウト — 後続の可変長データのバイト長（や要素数）をヘッダーが保持する形 — は、`valueof=` を付けた長さフィールドと、その対象フィールドに付けた `[len]` サイズ式のペアで表現します。各ペアは自動的に同期します。エンコード時は `valueof` が長さを埋め、デコード時はサイズ式がそれを読み取ります。

```go
type Record struct {
	Magic      uint32 `binary:"uint32"`                        // 自分で設定する
	NameLen    uint16 `binary:"uint16,valueof=bytelen(Name)"`  // 自動 = Name のエンコード後バイト長
	PayloadLen uint32 `binary:"uint32,valueof=bytelen(Payload)"`
	ItemCount  uint16 `binary:"uint16,valueof=count(Items)"`   // 自動 = Items の要素数

	Name    []byte   `binary:"[NameLen]byte"`     // デコード時に NameLen からサイズが決まる
	Payload []byte   `binary:"[PayloadLen]byte"`  // PayloadLen からサイズが決まる
	Items   []uint32 `binary:"[ItemCount]uint32"` // ItemCount からサイズが決まる
}

// エンコード: データフィールドだけを設定すれば、長さ・要素数フィールドは自動計算される。
rec := Record{Magic: 0x5A45, Name: []byte("file.txt"), Payload: data, Items: ids}
blob, _ := binarystruct.NewMarshalerOrder(binarystruct.LittleEndian).Marshal(&rec)
```

エンコード時に設定するのは `Name`・`Payload`・`Items` だけで、`NameLen`・`PayloadLen`・`ItemCount` は実データから書き込まれます。デコード時はサイズ式が各フィールドを正確な長さで読み戻します。（`valueof` は emit-only のため、`Marshal` 後もメモリ上の `rec.NameLen` は `0` のままです。構造体に値を反映したい場合は `Unmarshal` でラウンドトリップしてください。）

---

## 固定値・マジックナンバー（`const`）

`const` オプションはフィールドを固定値に固定します。**エンコード時に書き込み**（Go のフィールド値は無視）、**デコード時に検証**します（不一致なら `ErrValidationError`）。フォーマットのシグネチャやバージョンマーカーに最適です。

```go
type PNGHeader struct {
	Magic [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"` // \x89PNG\r\n\x1a\n
}
```

整数マジック（`const=0x04034b50`）はエンディアン依存なので、決定的にするには `endian=` を付けます。`[N]byte`/`string(N)` のバイト列マジックは自然なバイト順で書き込まれ、エンディアンに依存しません。どちらもコード生成に対応しています。詳細は[構造体タグリファレンス](STRUCT_TAGS_ja.md#9-固定値マジックナンバーconst)を参照してください。

`const` は `valueof` と併用できず、バイト列形式は `encoding=` と併用できません。両方の形式とも静的コードジェネレータでサポートされます。詳細は[構造体タグリファレンス](STRUCT_TAGS_ja.md#9-固定値マジックナンバーconst)を参照してください。

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
layout, _ := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Inspect(&pkt)

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

（カスタムコーデックやエンコーディングを使う構造体では、マーシャルに使うのと同じ設定済み `Marshaler` で `Inspect` を呼んでください。）レイアウトは `layout.ToJSON()` で JSON スキーマにも出力でき、ツール連携や他言語での型生成に便利です。

---

## プロダクション向けの静的コード生成（Codegen）

最大のパフォーマンスを得るには、スタンドアロンの **[`binarystruct-codegen`](binarystruct-codegen/README.md)** ツールが構造体タグから静的な `MarshalBinary`/`UnmarshalBinary` メソッドを生成し、実行時のリフレクションとレイアウト解析を排除します。ツールをインストールし `//go:generate` ディレクティブを追加すれば、`binarystruct.Marshal`/`Unmarshal` は自動的に生成コードへファストパスします。インストール・フラグ・対応機能・`go:generate` の使い方は **[コードジェネレータガイド](binarystruct-codegen/README.md)** を参照してください。

### 性能比較

以下の表は、コミット済みのクロスモード・ベンチマークスイート [`bench/`](bench) によって生成されます。同一の構造体を **safe** ランタイム、**unsafe** ランタイム（デフォルト）、**静的コード生成** の各経路でエンコード／デコードし、代表的な4つの形状（フラットなスカラー `Header`、1024要素の `IntSlice`、可変長の `Record`、ネスト構造体スライス `Nested`）で測定したものです。お使いのハードウェアでは `make bench` で再生成してください。

<!-- BENCH:START (generated by `make bench` — do not edit by hand) -->
| ワークロード | 処理 | Safe（ランタイム） | Unsafe（ランタイム） | コード生成 | コード生成の高速化 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Header** | Marshal | 402 ns / 4 allocs | 361 ns / 4 allocs | 228 ns / 3 allocs | 1.6× |
| **Header** | Unmarshal | 463 ns / 4 allocs | 410 ns / 4 allocs | 199 ns / 2 allocs | 2.1× |
| **IntSlice** | Marshal | 7,015 ns / 6 allocs | 5,906 ns / 6 allocs | 6,080 ns / 5 allocs | 1.0× |
| **IntSlice** | Unmarshal | 7,604 ns / 6 allocs | 6,864 ns / 6 allocs | 6,645 ns / 4 allocs | 1.0× |
| **Record** | Marshal | 635 ns / 6 allocs | 515 ns / 6 allocs | 378 ns / 5 allocs | 1.4× |
| **Record** | Unmarshal | 702 ns / 6 allocs | 569 ns / 6 allocs | 413 ns / 5 allocs | 1.4× |
| **Nested** | Marshal | 4,297 ns / 71 allocs | 3,956 ns / 71 allocs | 3,901 ns / 70 allocs | 1.0× |
| **Nested** | Unmarshal | 4,791 ns / 69 allocs | 4,278 ns / 69 allocs | 3,985 ns / 67 allocs | 1.1× |

> go1.26.3 にてこのマシンで `make bench` により測定（実行の平均）。数値はハードウェア依存です。**お使いの環境では `make bench` を再実行してください。** 数値は小さいほど高速。高速化率 = unsafe ÷ codegen。
<!-- BENCH:END -->

---

## バイトオフセット付きの詳細なエラーレポート

バイナリデータのデシリアライズ（Unmarshal）中にエラー（予期しないEOFなど）が発生した場合、エラーはカスタム構造体 `DecodeError` にラップされて返されます。これにより、失敗が発生した正確なバイトオフセットとフィールド名を特定できます：

```go
_, err := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Unmarshal(corruptedData, &pkt)
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

