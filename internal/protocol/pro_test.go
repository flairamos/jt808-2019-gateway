package protocol

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func Test1(t *testing.T) {
	// message := "7e0200403201000000000133074285530cc400000000000800030130dd3f06925ddf002b0000014f25052109462101040000033303020000250400000000300112310116cc7e"

	message := "7e8001400501000000000133074285530cc40cc4020001c47e"
	id, s, err := ParsePaketMessageId(message)
	fmt.Println(id, s, err)

	a1 := calculateChecksum(s)
	fmt.Println(a1)

}

func Test2(t *testing.T) {
	message := "7e0200403201000000000133074285530cc400000000000800030130dd3f06925ddf002b0000014f25052109462101040000033303020000250400000000300112310116cc7e"
	id, s, err := ParsePaketMessageId(message)
	if err != nil {
		fmt.Println("1", id, s, err)
	}
}

func Test3(t *testing.T) {
	message := "7e0102000a000123456789000130313233343536373839807e"
	idV2013, s, err := ParsePaketMessageIdV2013(message)
	fmt.Println(idV2013, s, err)
	v2013 := CommonReplyV2013(idV2013, 0)
	fmt.Println(v2013)
	fmt.Println(hex.EncodeToString(v2013))
}
