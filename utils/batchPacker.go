package utils

// BatchPacker 用于将元素分批打包, 通过回调函数实现
type BatchPacker[T any] struct {
	bSize int
	buf   []T
	f     func([]T)
}

func NewBatchPacker[T any](bSize int, f func([]T)) *BatchPacker[T] {
	bp := &BatchPacker[T]{bSize: bSize, f: f}
	bp.Reset()
	return bp
}

func (bp *BatchPacker[T]) Reset() {
	bp.buf = make([]T, 0, bp.bSize)
}

func (bp *BatchPacker[T]) Pack() {
	if len(bp.buf) > 0 {
		bp.f(bp.buf)
		bp.Reset()
	}
}

// Append appends elements to buffer and packs if necessary
func (bp *BatchPacker[T]) Append(v ...T) {
	for _, vv := range v {
		bp.buf = append(bp.buf, vv)
		if len(bp.buf) == bp.bSize {
			bp.Pack()
		}
	}
}
