package protocol

import (
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
