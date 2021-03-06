// +build windows

/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Test if various paths work as expected when converted to UNC form
func TestUNCPaths(t *testing.T) {
	var testCases = []struct {
		objName string
		pass    bool
	}{
		{"/abcdef", true},
		{"/a/b/c/d/e/f/g", true},
		{string(bytes.Repeat([]byte("界"), 85)), true},
		// Each path component must be <= 255 bytes long.
		{string(bytes.Repeat([]byte("界"), 100)), false},
		{`\\p\q\r\s\t`, true},
	}
	// Instantiate posix object to manage a disk
	var err error
	err = os.Mkdir("c:\\testdisk", 0700)

	// Cleanup on exit of test
	defer os.RemoveAll("c:\\testdisk")

	var fs StorageAPI
	fs, err = newPosix("c:\\testdisk")
	if err != nil {
		t.Fatal(err)
	}

	// Create volume to use in conjunction with other StorageAPI's file API(s)
	err = fs.MakeVol("voldir")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range testCases {
		err = fs.AppendFile("voldir", test.objName, []byte("hello"))
		if err != nil && test.pass {
			t.Error(err)
		} else if err == nil && !test.pass {
			t.Error(err)
		}

		fs.DeleteFile("voldir", test.objName)
	}
}

// Test to validate posix behaviour on windows when a non-final path component is a file.
func TestUNCPathENOTDIR(t *testing.T) {
	var err error
	// Instantiate posix object to manage a disk
	err = os.Mkdir("c:\\testdisk", 0700)

	// Cleanup on exit of test
	defer os.RemoveAll("c:\\testdisk")
	var fs StorageAPI
	fs, err = newPosix("c:\\testdisk")
	if err != nil {
		t.Fatal(err)
	}

	// Create volume to use in conjunction with other StorageAPI's file API(s)
	err = fs.MakeVol("voldir")
	if err != nil {
		t.Fatal(err)
	}

	err = fs.AppendFile("voldir", "/file", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// Try to create a file that includes a file in its path components.
	// In *nix, this returns syscall.ENOTDIR while in windows we receive the following error.
	err = fs.AppendFile("voldir", "/file/obj1", []byte("hello"))
	winErr := "The system cannot find the path specified."
	if !strings.Contains(err.Error(), winErr) {
		t.Errorf("expected to recieve %s, but received %s", winErr, err.Error())
	}
}

// Test to validate that path name in UNC form works
func TestUNCPathDiskName(t *testing.T) {
	var err error
	// Instantiate posix object to manage a disk
	longPathDisk := `\\?\c:\testdisk`
	err = mkdirAll(longPathDisk, 0777)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup on exit of test
	defer removeAll(longPathDisk)
	var fs StorageAPI
	fs, err = newPosix(longPathDisk)
	if err != nil {
		t.Fatal(err)
	}

	// Create volume to use in conjunction with other StorageAPI's file API(s)
	err = fs.MakeVol("voldir")
	if err != nil {
		t.Fatal(err)
	}
}

// Test to validate 32k path works on windows platform
func Test32kUNCPath(t *testing.T) {
	var err error
	// Instantiate posix object to manage a disk
	longDiskName := `\\?\c:`
	for {
		compt := strings.Repeat("a", 255)
		if len(compt)+len(longDiskName)+1 > 32767 {
			break
		}
		longDiskName = longDiskName + `\` + compt
	}

	if len(longDiskName) < 32767 {
		// The following calculation was derived empirically. It is not exactly MAX_PATH - len(longDiskName)
		// possibly due to expansion rules as mentioned here -
		// https://msdn.microsoft.com/en-us/library/windows/desktop/aa365247(v=vs.85).aspx#maxpath
		remaining := 32767 - 25 - len(longDiskName)
		longDiskName = longDiskName + `\` + strings.Repeat("a", remaining)
	}
	err = mkdirAll(longDiskName, 0777)
	if err != nil {
		t.Fatal(err)
	}

	// Cleanup on exit of test
	defer removeAll(longDiskName)
	_, err = newPosix(longDiskName)
	if err != nil {
		t.Fatal(err)
	}
}
