package drivers

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
	"io/ioutil"
	"os"
	"errors"
	"strings"
)

const (
	CephOptions = "cephopts"
)

type cephDriver struct {
	volumeDriver
	username   string
	password   string
	context    string
	cephmount  string
	cephport   string
	localmount string
	cephopts   map[string]string
}

//var (
//	EmptyMap = map[string]string{}
//)

func NewCephDriver(root string, username string, password string, context string, cephmount string, cephport string, localmount string, cephopts string) cephDriver {
	d := cephDriver{
		volumeDriver: newVolumeDriver(root),
		username:     username,
		password:     password,
		context:      context,
		cephmount:    cephmount,
		cephport:     cephport,
		localmount:   localmount,
		cephopts:     map[string]string{},
	}

	if len(cephopts) > 0 {
		d.cephopts[CephOptions] = cephopts
	}

        // Rebuild any existing volume references
        d.mountm.BuildReferences(root, CEPH.String())

	return d
}

func (n cephDriver) IsKeyPresent(keyName string) int {
	ret, keyval := keyctl_search("ceph", keyName)
	if ret != 0 {
		log.Debugf("Returned value = %d for key name: %s, value = %d", keyval, keyName, ret)
	}
	return ret
}

func (n cephDriver) UnlinkKey(keyName string) {
	ret, keyval := keyctl_search("ceph", keyName)
	if ret != 0 {
		keyctl_unlink(keyval)
	}
}

func (n cephDriver) Mount(r volume.MountRequest) volume.Response {
	log.Debugf("Entering Mount: %v", r)
	n.m.Lock()
	defer n.m.Unlock()
	hostdir := mountpoint(n.root, r.Name)
	source := n.fixSource(r.Name, r.ID)

	if n.mountm.HasMount(r.Name) && n.mountm.Count(r.Name) > 0 {
		log.Infof("Using existing CEPH volume mount: %s", hostdir)
		n.mountm.Increment(r.Name)
		return volume.Response{Mountpoint: hostdir}
	}

	n.mountm.Add(r.Name, hostdir)

	log.Infof("Mounting CEPH volume %s on %s", source, hostdir)
	if err := createDest(hostdir); err != nil {
		return volume.Response{Err: err.Error()}
	}

	if err := n.mountVolume(r.Name, source, hostdir); err != nil {
		return volume.Response{Err: err.Error()}
	}
	return volume.Response{Mountpoint: hostdir}
}

func (n cephDriver) Unmount(r volume.UnmountRequest) volume.Response {
	log.Debugf("Entering Unmount: %v", r)

	n.m.Lock()
	defer n.m.Unlock()
	hostdir := mountpoint(n.root, r.Name)

	if n.mountm.HasMount(r.Name) {
		if n.mountm.Count(r.Name) > 1 {
			log.Printf("Skipping unmount for %s - in use by other containers", r.Name)
			n.mountm.Decrement(r.Name)
			return volume.Response{}
		}
		n.mountm.Decrement(r.Name)
	}

	log.Infof("Unmounting volume name %s from %s", r.Name, hostdir)

	if err := run(fmt.Sprintf("umount %s", hostdir)); err != nil {
		return volume.Response{Err: err.Error()}
	}

	n.mountm.DeleteIfNotManaged(r.Name)

	n.UnlinkKey("client.cephFS");

	if err := os.RemoveAll(hostdir); err != nil {
		return volume.Response{Err: err.Error()}
	}

	return volume.Response{}
}

func (n cephDriver) fixSource(name, id string) string {
	if n.mountm.HasOption(name, ShareOpt) {
		return n.mountm.GetOption(name, ShareOpt)
	}
	source := strings.Split(name, "/")
	source[0] = source[0] + ":" + n.cephport + ":"
	return strings.Join(source, "/")
}

// read secret key from the specified file
func (n cephDriver) readSecretFile(secretFile string) string {
	data, err := ioutil.ReadFile(secretFile)
	if err != nil {
		log.Infof("readSecretFile: failed to read file: %s", secretFile)
	}

	return string(data)
}


// Function Name: mountVolume
//
// A ceph volume can be mounted in couple ways:
// i) starting docker-volume-netshare with secret passed in when started
// ii) starting docker-volume-netshare without secret
//	creating a volume with secret options.
//	For this to work, the expected syntax for passing optional parameters is:
//	 --opt addr=<addr> --opt name=<name> --opt secret=<secretKey> --opt device=:<mountPath> \
//		[--opt secretfile=<filename>] [--opt port=<cephPort>]
//	Note: If port is not specified, default value of 6789 is assumed.
//		Either secret or secretfile option must be specified.
func (n cephDriver) mountVolume(name, source, dest string) error {
	var cmd string

	options := n.mountOptions(n.mountm.GetOptions(name))
	opts := ""
	if val, ok := options[CephOptions]; ok {
		fmt.Println("opts = ", val)
		opts = "-o " + val
	}

	mountCmd := "mount"

	username := ""
	if len(options["name"]) != 0 {
		username = "name=" + options["name"]
	} else {
		username = n.username
	}

	if (len(options["secretfile"]) != 0  && len(options["secret"]) != 0) {
		return errors.New("Cannot pass secret and secretfile options together")
	}

	passwd := n.password
	if len(options["secretfile"]) != 0 {
		passwd = "secret=" + n.readSecretFile(options["secretfile"])
	}

	if len(options["secret"]) != 0 {
		passwd = "secret=" + options["secret"]
	}

	cephPort := ""
	if len(options["port"]) != 0 {
		cephPort = ":" + options["port"]
	} else {
		cephPort = ":6789"
	}

	srcDir := ""
	if len(options["addr"]) != 0 {
		// Ceph Source IP addresses: can be comma separated list
		//	[IP_Address][,IP_Address]+
		// The mount should look like this:
		//	addr1:6789[,addrN:6789]
		addrList := strings.Split(options["addr"], ",")
		for k := 0; k < len (addrList); k++ {
			srcDir += addrList[k] + cephPort
			if (k +1) < len(addrList) {
				srcDir += ","
			}
		}
		srcDir += options["device"]
	} else {
		srcDir = source
	}

	//cmd = fmt.Sprintf("%s -t ceph %s:%s:/ -o %s,%s,%s %s %s", mountCmd, n.cephmount, n.cephport, n.context, n.username, n.password, opts, dest)
	// This add -t ceph twice for some reason which is actually harmless
	if log.GetLevel() == log.DebugLevel {
		mountCmd = mountCmd + " -t ceph"
		cmd = fmt.Sprintf("%s %s -o \"%s,%s,%s %s\" %s", mountCmd, srcDir, n.context, username, passwd, opts, dest)
	} else {
		cmd = fmt.Sprintf("%s -t ceph %s -o \"%s,%s,%s\" %s %s", mountCmd, srcDir, n.context, username, passwd, opts, dest)
	}

	log.Debugf("exec: %s\n", strings.Replace(cmd, ","+passwd, ",****", 1))

	return run(cmd)
}

func (n cephDriver) mountOptions(src map[string]string) map[string]string {
	if len(n.cephopts) == 0 && len(src) == 0 {
		return EmptyMap
	}

	dst := map[string]string{}
	for k, v := range n.cephopts {
		dst[k] = v
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
