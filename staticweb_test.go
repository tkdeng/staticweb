package staticweb

import "testing"

func Test(t *testing.T) {
	err := Compile("./test/src", "./test/dist")
	if err != nil {
		t.Error(err)
	}
}
