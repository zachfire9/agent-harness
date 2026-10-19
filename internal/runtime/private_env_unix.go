//go:build !windows

package runtime

import (
	"fmt"
	"os"
)

func checkPrivateEnvironmentFile(_ string, info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private environment file must have mode 0600")
	}
	return nil
}
