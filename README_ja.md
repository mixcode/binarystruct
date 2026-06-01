[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct)

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

* **自動的な型変換と範囲チェック**: タグ記述に基づいての型の変換と、安全な範囲チェック。
* **単一値のエンコード/デコード**: 構造体ではない変数も [MarshalAs](https://pkg.go.dev/github.com/mixcode/binarystruct#MarshalAs) と [UnmarshalAs](https://pkg.go.dev/github.com/mixcode/binarystruct#UnmarshalAs) で直接シリアライズ可能。
* **明示的なエンディアン指定**: 構造体フィールドごとにエンディアンを指定可能（例: `binary:"uint16,endian=inverse"` や `endian=big|little`）。
* **デフォルトテキストエンコーディング**: [Marshaller](https://pkg.go.dev/github.com/mixcode/binarystruct#Marshaller) にデフォルトのエンコーディングを指定可能。
* **拡張されたタグ内計算式**: `+`, `-`, `*`, `/` およびかっこ `()` を含む数式評価をサポート。
* **カスタムシリアライザ**: `Serializer` インターフェースを実装した独自シリアライザを `serializer=Name` オプションで適用可能。
* **構造体メタデータのキャッシュ処理**: 正規表現などのパースをキャッシュ化し、シリアライズ時のパフォーマンスを大幅に向上。
* **Unsafe型構造体インタプリタおよびスライスの高速処理パス**: `unsafe.Pointer` を用いてリフレクションを回避し、さらに同一レイアウト数値スライスのダイレクト処理により最大 214 倍の超高速化と 99.9% のアロケーション削減を実現。

## 動作モード（Safe vs. Unsafe / SIMD）

パフォーマンス要件や実行環境の制約、実験的なハードウェア支援に合わせた複数のビルドモードをサポートしています。

| モード / ビルドタグ | 概要 | パフォーマンス・特徴 |
| :--- | :--- | :--- |
| **デフォルト（Unsafe）** | `unsafe.Pointer` インタプリタとレイアウト適合スライスの高速処理パスを用いて、リフレクションなしで直接メモリアドレスにアクセスします。 | **最高速度**（最大214倍高速、メモリ割り当てを99.9%削減）。 |
| **Safeモード** (`-tags safe`) | 純粋なリフレクションのみを用いる標準Go実装にフォールバックします。Google App Engineなどのセキュリティ上の制限がある環境で必須。 | リフレクションによる標準的なオーバーヘッド。 |
| **SIMDモード** (`GOEXPERIMENT=simd -tags experiment_simd`) | Go 1.26 の実験的パッケージ `simd/archsimd` を用いて、AMD64上でのエンディアン変換（バイトスワップ）をベクター命令で処理します（CPU機能検知付き）。 | 大きな数値配列やスライスのベクター化によるスループット最大化。 |

## ドキュメント
詳細は[Goドキュメント](https://pkg.go.dev/github.com/mixcode/binarystruct) をご覧ください。

