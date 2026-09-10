package slicesyntax

import (
	"fmt"
	"strconv"
	"strings"

	"NothingBot_v4/utils"
)

type SliceSyntaxes []SliceSyntax

func (sss SliceSyntaxes) String() string {
	if len(sss) == 0 {
		return ""
	}
	res := make([]string, len(sss))
	for i, ss := range sss {
		res[i] = ss.String()
	}
	return strings.Join(res, ",")
}

// ToIndexes 将多个 SliceSyntax 转换为索引
func (sss SliceSyntaxes) ToIndexes(sliceLen int) (indexes []int) {
	if len(sss) == 0 {
		return nil
	}
	indexes = []int{}
	for _, ss := range sss {
		indexes = append(indexes, ss.ToIndexes(sliceLen)...)
	}
	return indexes
}

// ToIndexesNoRepeat 将多个 SliceSyntax 转换为索引, 会去重
func (sss SliceSyntaxes) ToIndexesNoRepeat(sliceLen int) (indexes []int) {
	if len(sss) == 0 {
		return nil
	}
	set := utils.Set[int]{}
	for _, ss := range sss {
		indexes = append(indexes, set.Clean(ss.ToIndexes(sliceLen))...)
	}
	return indexes
}

type SliceSyntax []int // len 1: index, len 2: range

func (ss SliceSyntax) String() string {
	switch len(ss) {
	case 1:
		return fmt.Sprintf("[%d]", ss[0])
	case 2:
		switch {
		case ss[0] == 0 && ss[1] == 0:
			return "[:]"
		case ss[0] == 0:
			return fmt.Sprintf("[:%d]", ss[1])
		case ss[1] == 0:
			return fmt.Sprintf("[%d:]", ss[0])
		default:
			return fmt.Sprintf("[%d:%d]", ss[0], ss[1])
		}
	}
	return ""
}

// ToIndexes 将 SliceSyntax 转换为索引列,
//
// 负数索引会被转换为正数索引, 例如 -1 会被转换为 sliceLen-1
func (ss SliceSyntax) ToIndexes(sliceLen int) (indexes []int) {
	switch len(ss) {
	case 1:
		index := ss[0]
		if index < 0 {
			index += sliceLen
		}
		indexes = []int{index}
	case 2:
		start, end := ss[0], ss[1]
		if start < 0 {
			start += sliceLen
		}
		if end < 0 {
			end += sliceLen
		}
		if start > end {
			start, end = end, start
		}
		indexes = make([]int, 0, end-start)
		for i := range end - start {
			if start+i < sliceLen { // 防止越界
				indexes = append(indexes, start+i)
			}
		}
	}
	return
}

// ParseMulti 解析多个切片语法, 不限制分隔符
func ParseMulti(str string) (sss SliceSyntaxes) {
	stack := utils.Stack[int]{} // 泛型栈 存储 '[' 的索引
	for i, char := range str {
		switch char {
		case '[':
			stack.Push(i)

		case ']':
			start, ok := stack.Pop()
			if !ok {
				continue
			}

			// 提取包括中括号在内的内容
			content := str[start : i+1]
			sss = append(sss, Parse(content))

		}
	}
	return sss
}

// Parse 解析切片语法,
//
// 只支持 [n:m] / [:m] / [n:] / [n] 的格式,
//
// 其中 n, m 可以是负数, 代表从后往前索引,
//
// 传入的字符串需要带上中括号
func Parse(str string) (sss SliceSyntax) {
	if str[0] != '[' || str[len(str)-1] != ']' {
		return nil
	}
	if strings.Contains(str, ":") {
		splits := strings.Split(str, ":")
		if len(splits) != 2 {
			return nil
		}
		// [n -> n, m] -> m
		s, e := splits[0][1:], splits[1][:len(splits[1])-1]
		var start, end int
		var err error
		if s != "" {
			start, err = strconv.Atoi(s)
			if err != nil {
				return nil
			}
		}
		if e != "" {
			end, err = strconv.Atoi(e)
			if err != nil {
				return nil
			}
		}
		return SliceSyntax{start, end}

	} else {
		str = str[1 : len(str)-1]
		index, err := strconv.Atoi(str)
		if err != nil {
			return nil
		}
		return SliceSyntax{index}
	}
}

// DoIndexes is possible to panic
func DoIndexes[T any](slice []T, indexes []int) []T {
	if len(indexes) == 0 {
		return nil
	}
	res := make([]T, len(indexes))
	for i, index := range indexes {
		res[i] = slice[index]
	}
	return res
}
