package aws_account

func ptr[T any](t T) *T {
	return &t
}

func unptr[T any](t *T) T {
	var result T
	if t != nil {
		result = *t
	}
	return result
}
