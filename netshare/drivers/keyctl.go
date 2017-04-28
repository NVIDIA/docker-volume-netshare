package drivers

import (
	log "github.com/Sirupsen/logrus"
	"syscall"
	"unsafe"
)

type keyctlCommand int
const syscall_keyctl uintptr = 250
const keySpecUserKeyring int32 = -4
const keyctlUnlink keyctlCommand = 9
const keyctlSearch keyctlCommand = 10

func keyctl(cmd keyctlCommand, args ...uintptr) (r1 int32, r2 int32, err error) {
	a := make([]uintptr, 6)
	l := len(args)
	if l > 5 {
		l = 5
	}
	a[0] = uintptr(cmd)
	for idx, v := range args[:l] {
		a[idx+1] = v
	}

	v1, v2, errno := syscall.Syscall6(syscall_keyctl, a[0], a[1], a[2], a[3], a[4], a[5])
	if errno != 0 {
		log.Infof("syscall keyctl for cmd:%d failed.", cmd)
		err = errno
		return
	}

	r1 = int32(v1)
	r2 = int32(v2)
	return
}

func keyctl_search(name, keyName string) (result int, keyValue int32) {
	var (
		r1	int32
		b1, b2	*byte
		err1	error
	)

	result = 0
	// For e.g. name is "ceph"
	if b1, err1 = syscall.BytePtrFromString(name); err1 != nil {
		log.Debugf("keyctl_search: failed to convert name to byte-ptr\n")
		return
	}
	// For e.g. keyName is "client.cephFS"
	if b2, err1 = syscall.BytePtrFromString(keyName); err1 != nil {
		log.Debugf("keyctl_search: failed to convert keyName to byte-ptr\n")
		return
	}

	r1 = int32(keySpecUserKeyring) /* always uses User Spec */
	r0, _, err1 := keyctl(keyctlSearch, uintptr(r1), uintptr(unsafe.Pointer(b1)), uintptr(unsafe.Pointer(b2)))
	if err1 != nil {
		log.Debugf("keyctl_search: failed to find key: %s\n", keyName)
		return
	} 

	keyValue = int32(r0)
	result = 1
	log.Debugf("keyctl_sarch: Found Key with value: %v", keyValue)
	return
}

func keyctl_unlink(keyValue int32) {
	r1 := int32(keySpecUserKeyring)
	if r0, _, err2 := keyctl(keyctlUnlink, uintptr(keyValue), uintptr(r1)); err2 != nil {
		log.Debugf("keyctl_unlink: failed to unlink keyValue: %s, value: %d\n", keyValue, r0)
		return
	}

	log.Debugf("Unlinked Key: %v", keyValue)
	return
}
