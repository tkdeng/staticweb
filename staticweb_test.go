package staticweb

import (
	"os"
	"testing"
)

func Test(t *testing.T) {
	os.RemoveAll("./test/dist")
	err := Compile("./test/src", "./test/dist")
	if err != nil {
		t.Error(err)
	}
}
