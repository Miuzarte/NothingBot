package utils

import (
	"fmt"

	"github.com/jinzhu/copier"
)

// AnyCopy 复制 any 到 any
func AnyCopy[T any](from any) (to *T) {
	to = new(T)
	err := copier.Copy(to, from)
	if err != nil {
		panic(fmt.Sprintf("failed to copy from %T to %T: %v", from, to, err))
	}
	return to
}

func Hyperlink(link string) string {
	return fmt.Sprintf("\x1b]8;;file://%s\x1b\\%s\x1b]8;;\x1b\\", link, link)
}
