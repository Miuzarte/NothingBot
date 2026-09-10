package utils

import "unsafe"

// ToStringG 没有复制开销的字节切片转字符串
func ToStringG[T ~[]byte](b T) string {
	return *(*string)(unsafe.Pointer(&b))
}

// ToString 没有复制开销的字节切片转字符串
func ToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// ToBytesG 没有复制开销的字符串转字节切片
//
// []byte(s) 本来就没有开销, 除非转完后修改了 bytes 的内容
func ToBytesG[T ~string](s T) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

// ToBytesG 没有复制开销的字符串转字节切片
//
// []byte(s) 本来就没有开销, 除非转完后修改了 bytes 的内容
func ToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
