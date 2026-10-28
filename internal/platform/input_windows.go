package platform

import "io"

func PrepareProcessInput(input io.Reader) (io.ReadCloser, error) {
	if closer, ok := input.(io.ReadCloser); ok {
		return closer, nil
	}
	return io.NopCloser(input), nil
}
