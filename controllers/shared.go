package controllers

const finalizerName = "pingone.io/cleanup"

func containsFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, finalizer string) []string {
	out := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != finalizer {
			out = append(out, f)
		}
	}
	return out
}
