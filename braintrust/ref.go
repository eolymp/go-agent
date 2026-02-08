package braintrust

func Ref[V any](v V) *V {
	return &v
}

func Deref[V any](v *V) V {
	if v != nil {
		return *v
	}

	var zero V
	return zero
}
