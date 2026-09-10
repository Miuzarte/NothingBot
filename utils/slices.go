package utils

import "slices"

// DelElements 删除切片中的元素
func DelElements[T comparable](slice []T, elems ...T) []T {
	return slices.DeleteFunc(slice, func(e T) bool {
		return slices.Contains(elems, e)
	})
}
