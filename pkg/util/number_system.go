package util

import (
	"strconv"
	"strings"
)

// 十六进制字符串转换为十进制整数
func HexToDecimal(hexStr string) (int64, error) {
	// 使用 strconv.ParseInt 函数将十六进制字符串转换为十进制整数
	// 第二个参数 16 表示输入的字符串是十六进制格式
	// 第三个参数 64 表示将结果解析为 64 位整数
	return strconv.ParseInt(hexStr, 16, 64)
}

// 二进制转十进制
func BinaryToDecimal(binaryStr string) (int, error) {
	// 将二进制字符串转换为整数
	num, err := strconv.ParseInt(binaryStr, 2, 64)
	if err != nil {
		return 0, err
	}
	return int(num), nil
}

// HexToBinary 将十六进制字符串转换为二进制字符串
func HexToBinary(hexStr string) (string, error) {
	// 将十六进制字符串转换为整数
	num, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return "", err
	}
	// 将整数转换为二进制字符串
	binaryStr := strconv.FormatInt(num, 2)
	// 补齐前导零
	length := 8 - len(binaryStr)
	for i := 0; i < length; i++ {
		binaryStr = "0" + binaryStr
	}
	return binaryStr, nil
}

// BinStrToHexStr 二进制字符串 -> 大写16进制字符串
// 例："0000000000000101" → "0005"
func BinStrToHexStr(binStr string) string {
	// 把二进制字符串解析为大整数
	val, err := strconv.ParseUint(binStr, 2, 64)
	if err != nil {
		return ""
	}
	// 转为16进制大写
	hexStr := strconv.FormatUint(val, 16)
	return strings.ToLower(hexStr)
}
