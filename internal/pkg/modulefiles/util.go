package modulefiles

func join[T any](arrs ...[]T) []T {
	size := 0
	for _, v := range arrs {
		size += len(v)
	}
	i := 0
	dst := make([]T, size)
	for _, arr := range arrs {
		for _, e := range arr {
			dst[i] = e
			i++
		}
	}
	return dst
}
