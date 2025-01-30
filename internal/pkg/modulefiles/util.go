package modulefiles

func applyNested[T any](f func(T), arrs ...[]T) {
	for _, arr := range arrs {
		for _, e := range arr {
			f(e)
		}
	}
}
