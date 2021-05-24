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

* タグ記述に基づいての型の変換
* 可変長の配列・文字列サポート。長さに構造体の他のメンバーの値を指定できる
* 文字列のテキストエンコーディング指定

## ドキュメント
詳細は[Goドキュメント](https://pkg.go.dev/github.com/mixcode/binarystruct) をご覧ください。

