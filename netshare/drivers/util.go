package drivers

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ShareSplitIndentifer = "#"
)

func createDest(dest string) error {
	fi, err := os.Lstat(dest)

	if os.IsNotExist(err) {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return fmt.Errorf("mkdir failed for %v with error: '%s'", dest, err.Error())
			return err
		}
	} else if err != nil {
		return fmt.Errorf("lstat failed for %v with error: '%s'", dest, err.Error())
		return err
	}

	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("%v already exist and it's not a directory", dest)
	}
	return nil
}

// Used to support on the fly volume creation using docker run. If = is in the name we split
// and elem[1] is the volume name
func resolveName(name string) (string, map[string]string) {
	if strings.Contains(name, ShareSplitIndentifer) {
		sharevol := strings.Split(name, ShareSplitIndentifer)
		opts := map[string]string{}
		opts[ShareOpt] = sharevol[0]
		opts[CreateOpt] = "true"
		return sharevol[1], opts
	}
	return name, nil
}

func shareDefinedWithVolume(name string) bool {
	return strings.Contains(name, ShareSplitIndentifer)
}

func addShareColon(share string) string {
	if strings.Contains(share, ":") {
		return share
	}
	source := strings.Split(share, "/")
	source[0] = source[0] + ":"
	return strings.Join(source, "/")
}

func mountpoint(elem ...string) string {
	return filepath.Join(elem...)
}

func run(cmd string) error {
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func run_cmd(cmd string) error {
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		regex1 := regexp.MustCompile(".*access denied by server while mounting.*")
		regex2 := regexp.MustCompile(".*Failed to resolve server.*")
		regex3 := regexp.MustCompile(".*Device or resource busy.*")
		var error_string string
		if error_string = regex1.FindString(string(out)); error_string == "" {
			if error_string = regex2.FindString(string(out)); error_string == "" {
				if error_string = regex3.FindString(string(out)); error_string == "" {
					log.Println(string(out))
					return err
				}
			}
		}
		log.Println(string(out))
		return fmt.Errorf(string(out))
	}
	return nil
}

func merge(src, src2 map[string]string) map[string]string {
	if len(src) == 0 && len(src2) == 0 {
		return EmptyMap
	}

	dst := map[string]string{}
	for k, v := range src2 {
		dst[k] = v
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
