# 構造体タグ リファレンスマニュアル

`binarystruct` は、Go構造体のフィールドに `binary` タグを指定することで、バイナリデータレイアウトへのマッピング、自動型変換、動的なサイズ検証を行います。

---

## 1. 構文の概要

構造体タグは、**バイナリ型**（必要に応じて配列サイズ指定を前置、またはバッファサイズ指定を後置）と、カンマで区切られたゼロ個以上の**オプション**で構成されます。

```go
`binary:"[配列長]型名(バッファ長),オプション1=値1,オプション2"`
```

例：
```go
// shift-jis エンコーディングで、長さがフィールド "StrLen" の値+2となるオミット可能な文字列
MyString string `binary:"string(StrLen+2),encoding=shift-jis,omittable"`
```

---

## 2. バイナリ型（対応型一覧）

| タグ型名 | Goの型 | シリアライズサイズ | 概要 |
| :--- | :--- | :--- | :--- |
| **`int8`** | 符号付き整数 / bool | 1 バイト | 8ビット符号付き整数 |
| **`int16`** | 符号付き整数 | 2 バイト | 16ビット符号付き整数 |
| **`int32`** | 符号付き整数 | 4 バイト | 32ビット符号付き整数 |
| **`int64`** | 符号付き整数 | 8 バイト | 64ビット符号付き整数 |
| **`uint8`** | 符号なし整数 | 1 バイト | 8ビット符号なし整数 |
| **`uint16`** | 符号なし整数 | 2 バイト | 16ビット符号なし整数 |
| **`uint32`** | 符号なし整数 | 4 バイト | 32ビット符号なし整数 |
| **`uint64`** | 符号なし整数 | 8 バイト | 64ビット符号なし整数 |
| **`byte`** | 任意 | 1 バイト | 型非依存の8ビットビットマップ |
| **`word`** | 任意 | 2 バイト | 型非依存の16ビットビットマップ |
| **`dword`** | 任意 | 4 バイト | 型非依存の32ビットビットマップ |
| **`qword`** | 任意 | 8 バイト | 型非依存の64ビットビットマップ |
| **`float32`** | 浮動小数点 | 4 バイト | IEEE 754 32ビット単精度浮動小数点 |
| **`float64`** | 浮動小数点 | 8 バイト | IEEE 754 64ビット倍精度浮動小数点 |
| **`string`** | 文字列 / スライス | 可変 / `バッファ長` | 生のバイト文字列（バッファ長指定時は `0` でパディング） |
| **`bstring`** | 文字列 | 1 + len バイト | 長さプレフィックス付き文字列（1バイト長のプレフィックス） |
| **`wstring`** | 文字列 | 2 + len バイト | 長さプレフィックス付き文字列（2バイト長のプレフィックス） |
| **`dwstring`** | 文字列 | 4 + len バイト | 長さプレフィックス付き文字列（4バイト長のプレフィックス） |
| **`zstring`** | 文字列 | len + 1 バイト | ヌル終端文字列（C言語スタイル） |
| **`z16string`**| 文字列 | 2 * len + 2 バイト | ヌルワード終端文字列（UTF-16スタイルなど） |
| **`pad`** | なし | `バッファ長` バイト | ゼロで埋められるパディング。ソースの値は無視されます |
| **`ignore`** / **`-`** | 任意 | 0 バイト | シリアライズ/デシリアライズ時に対象外として無視されます |
| **`any`** | 任意 | 自然長 | Goのフィールドの型に合わせた標準的な変換を行います |
| **`custom`** | 任意 | カスタム | カスタムシリアライザの適用を示します（`serializer` オプションと併用） |

---

## 3. タグオプション

タグオプションはカンマ区切りで追加します：

### `encoding=NAME`
文字列変換用のテキストエンコーディングを指定します。
* **使用例**: `binary:"string(10),encoding=shift-jis"`
* `utf-8`、`shift-jis`、`euc-jp`、`utf-16le` などに対応（`Marshaller.AddTextEncoding` で登録可能）。

### `endian=big|little|inverse`
このフィールドのデフォルトエンディアン（バイトオーダー）を上書きします。
* **`big`**: ビッグエンディアンを強制。
* **`little`**: リトルエンディアンを強制。
* **`inverse`**: 親構造体に指定されたバイトオーダーを反転。
* **使用例**: `Value uint32 `binary:"uint32,endian=inverse"``（ネストされた構造体フィールドにも再帰的に伝播します）。

### `serializer=NAME`
登録済みのカスタム `Serializer` を用いて、このフィールドをエンコード/デコードします。
* **使用例**: `Data MyCustomType `binary:"custom,serializer=MyCustomSerializer"``

### `omittable[=Expr]`
フィールドをオミット可能（オプション）としてマークします。
* 式なしで `omittable` を指定した場合、デシリアライズ時にフィールドの開始時点で `io.EOF` を検出してもエラーにならず、静かに処理を完了します。
* 式付きで指定した場合（例: `omittable=LimitExpr`）、現在のバイト処理位置 `n` が評価値以上の場合に処理をスキップします。
* **使用例**: `Extra uint32 `binary:"uint32,omittable"``

---

## 4. 配列およびバッファサイズ表記

### 配列サイズ指定: `[長さ]型名`
フィールドが指定された配列サイズであることを示します。
* **使用例**: `Data []int `binary:"[10]int16"``
* Goの固定長配列（例: `[4]string`）を使用する場合、タグ側の配列長さは省略できます: `binary:"[]string(10)"`。

### 文字列バッファサイズ指定: `型名(バッファ長)`
文字列のバイトバッファを正確に `バッファ長` に制限・パディングします。
* **使用例**: `Name string `binary:"string(16)"``（16バイトより短い場合はゼロでパディングされ、長い場合は切り詰められます）。

---

## 5. 動的な計算式

配列の長さ `[長さ]` および文字列のバッファサイズ `(バッファ長)` には、定数だけでなく、**他の構造体フィールドを参照した動的な計算式**を使用できます。

* **使用可能な演算子**: `+`、`-`、`*`、`/` およびかっこ `()`。
* **フィールド参照**: 参照先フィールドの現在の値に基づいて動的に評価されます。

### 使用例
```go
type Packet struct {
	HeaderLength int    `binary:"uint8"`
	PayloadSize  int    `binary:"uint16"`
	
	// バッファサイズを他のフィールドの値を用いて動的に計算
	Payload      []byte `binary:"[PayloadSize - (HeaderLength * 2)]byte"`
}
```

---

## 6. インターフェースとポリモーフィズムの処理

`binarystruct` は、インターフェース型（`interface{}` / `any`）のフィールドに対して、以下の2つの方法でシリアライズおよびデシリアライズを行うことができます。

### 方法 1: 事前割り当て済みインターフェース（静的型決定）
構造体のフィールドがインターフェース型である場合、デコーダーは `Unmarshal` が呼び出される前にそのフィールドに**具体的な値が事前割り当て（pre-assigned）**されているかをチェックします。事前割り当てされている場合、デコーダーはその割り当てられている具象型を自動的に判定してデコードします。

```go
type Packet struct {
	Payload interface{} `binary:"any"` // 事前割り当てされた型のレイアウトとして解決される
}

// デコードされる予定の具体的な構造体をあらかじめ割り当てておく
var data int32 = 0
pkt := Packet{Payload: &data}

// Unmarshal はバイナリデータを 'data' 変数に直接デコードします
_, err := binarystruct.Unmarshal(blob, binarystruct.LittleEndian, &pkt)
```

### 方法 2: カスタムシリアライザによる動的割り当て
Type-Length-Value（TLV）や、パケットヘッダーの後に動的なメッセージボディが続くパケット形式のように、動的に具象型を割り当てたい場合は、カスタムの `Serializer` を使用します。

カスタムデシリアライザ内では、すでにデコード済みの親構造体のフィールド（例: `Type` や `MessageID` など）の値を検査して、実行時に動的に適切な具象型を割り当てることができます。

```go
type Packet struct {
	MsgType uint8       `binary:"uint8"`
	Payload interface{} `binary:"custom,serializer=DynamicPayload"`
}

func (s *DynamicPayloadSerializer) Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
	// 親構造体のすでにデコード済みの "MsgType" フィールドをチェックする
	msgTypeField := parentStruct.FieldByName("MsgType")
	
	// タイプ値に基づいて動的に構造体を割り当てる
	var payload interface{}
	switch msgTypeField.Uint() {
	case 1:
		payload = &MessageA{}
	case 2:
		payload = &MessageB{}
	}

	// 割り当てた構造体にバイナリストリームからデータをデコードする
	n, err = binarystruct.Read(r, order, payload)
	return payload, n, err
}
```

実際にコンパイルして動作確認可能な詳細なコード例は、[example_interface_test.go](example_interface_test.go) を参照してください。
