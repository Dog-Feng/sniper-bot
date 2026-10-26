package evm

import "testing"

func TestEventTopic(t *testing.T) {
	got := EventTopic("TokenLaunched(address,address,address,address,uint256,uint256)")
	want := "8d4aad4953d0ca700d468f3753aa14432d1b35b43ec6409f051fb6aa43a89607"
	if got.Raw() != want {
		t.Fatalf("topic0 = %s, want %s", got.Raw(), want)
	}
}

func TestFuncSelector(t *testing.T) {
	got := FuncSelector("supportsInterface(bytes4)")
	want := []byte{0x01, 0xff, 0xc9, 0xa7}
	if string(got) != string(want) {
		t.Fatalf("selector = %x, want %x", got, want)
	}
}

func TestParseAddress(t *testing.T) {
	a, err := ParseAddress("0x331dcbdfa2f5dfd140452311f42406d83734383f")
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != "0x331dcbdfa2f5dfd140452311f42406d83734383f" {
		t.Fatalf("addr = %s", a.String())
	}
}

func TestBytesToAddressLow20(t *testing.T) {
	// 32 字节 topic：地址右对齐（低 20 字节）。
	topic := "000000000000000000000000271d4be059a3d00142ff02a4097f6a5df35106b3"
	b, _ := DecodeHex(topic)
	a := BytesToAddress(b)
	want := "271d4be059a3d00142ff02a4097f6a5df35106b3"
	if a.Raw() != want {
		t.Fatalf("BytesToAddress = %s, want %s", a.Raw(), want)
	}
}

func TestDecodeUint256(t *testing.T) {
	slot := make([]byte, 32)
	slot[31] = 0x2a // 42
	v, err := DecodeUint256(slot)
	if err != nil {
		t.Fatal(err)
	}
	if v.Uint64() != 42 {
		t.Fatalf("value = %d", v.Uint64())
	}
}
