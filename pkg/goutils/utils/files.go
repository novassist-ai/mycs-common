package utils

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/otiai10/copy"
)

// Creates a symbolic link to the given src path. If
// windows then hard copy is done as some tools are
// unable to follow windows links
func LinkPaths(linkSrc, linkDest string) error {

	var (
		err error
	)

	if runtime.GOOS == "windows" {
		// terraform does not follow symlinks in
		// windows so make physical copy of provider					
		if err = copy.Copy(
			linkSrc, 
			linkDest, 
			copy.Options{
				OnSymlink: func(src string) copy.SymlinkAction {
					return copy.Deep
				},
			},
		); err != nil {
			return err
		}

	} else {
		os.Remove(linkDest)
		if err = os.Symlink(linkSrc, linkDest); err != nil {
			return err
		}
	}
	return nil
}

// Copies a source file to the given destination file 
// path. The function uses a buffer that optimizes the 
// copy operation. 
func CopyFiles(src, dst string, BUFFERSIZE int64) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file.", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	_, err = os.Stat(dst)
	if err == nil {
		return fmt.Errorf("File %s already exists.", dst)
	}

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if err != nil {
		panic(err)
	}

	buf := make([]byte, BUFFERSIZE)
	for {
		n, err := source.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := destination.Write(buf[:n]); err != nil {
			return err
		}
	}
	return err
}

// Searches from the given path for a directory of the given name.
// If the directory is found in the user's home directory then it is
// returned. If not found the the search continues up the directory
// tree. If directory is not found then it will be created at the
// given path.
func DirReverseLookup(name, homePath string) string {

	return ""
}

// DirCompare compares the digests of all files in the given directories
// and returns whether the directory contents match or not
func DirCompare(dirRoot1, dirRoot2 string) (bool, error) {
	
	var (
		err1, 
		err2 error

		dirRoot1Sums,
		dirRoot2Sums map[string][md5.Size]byte

		wg sync.WaitGroup
	)
	
	wg.Add(2)
	go func() {
		defer wg.Done()
		dirRoot1Sums, err1 = MD5DirAll(dirRoot1)
	}()
	go func() {
		defer wg.Done()
		dirRoot2Sums, err2 = MD5DirAll(dirRoot2)		
	}()
	wg.Wait()

	if err1 != nil {
		return false, err1
	}
	if err2 != nil {
		return false, err2
	}

	numFiles1 := len(dirRoot1Sums)
	numFiles2 := len(dirRoot2Sums)
	if numFiles1 == numFiles2 {
		for f, r1 := range dirRoot1Sums {
			ff := filepath.Join(dirRoot2, strings.TrimPrefix(f, dirRoot1))
			if r2, ok := dirRoot2Sums[ff]; ok {
				if !bytes.Equal(r1[:], r2[:]) {
					return false, nil
				}
				numFiles1--
			}
		}
		return numFiles1 == 0, nil	
	}

	return false, nil
}

// MD5All reads all the files in the file tree rooted at root and returns a map
// from file path to the MD5 sum of the file's contents.  If the directory walk
// fails or any read operation fails, MD5All returns an error.  In that case,
// MD5All does not wait for inflight read operations to complete.
func MD5DirAll(dirRoot string) (map[string][md5.Size]byte, error) {
	// MD5All closes the done channel when it returns; it may do so before
	// receiving all the values from c and errc.
	done := make(chan struct{}) // HLdone
	defer close(done)           // HLdone

	c, errc := sumFiles(done, dirRoot) // HLdone

	m := make(map[string][md5.Size]byte)
	for r := range c { // HLrange
		if r.err != nil {
			return nil, r.err
		}
		m[r.path] = r.sum
	}
	if err := <-errc; err != nil {
		return nil, err
	}
	return m, nil
}

// A sumResult is the product of reading and summing a file using MD5.
type sumResult struct {
	path string
	sum  [md5.Size]byte
	err  error
}

// sumFiles starts goroutines to walk the directory tree at root and digest each
// regular file.  These goroutines send the results of the digests on the result
// channel and send the result of the walk on the error channel.  If done is
// closed, sumFiles abandons its work.
func sumFiles(done <-chan struct{}, root string) (<-chan sumResult, <-chan error) {
	// For each regular file, start a goroutine that sums the file and sends
	// the result on c.  Send the result of the walk on errc.
	c := make(chan sumResult)
	errc := make(chan error, 1)
	go func() { // HL
		var wg sync.WaitGroup
		err := filepath.WalkDir(root, func(path string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !de.Type().IsRegular() {
				return nil
			}
			wg.Add(1)
			go func() { // HL
				data, err := os.ReadFile(path)
				select {
				case c <- sumResult{path, md5.Sum(data), err}: // HL
				case <-done: // HL
				}
				wg.Done()
			}()
			// Abort the walk if done is closed.
			select {
			case <-done: // HL
				return errors.New("walk canceled")
			default:
				return nil
			}
		})
		// Walk has returned, so all calls to wg.Add are done.  Start a
		// goroutine to close c once all the sends are done.
		go func() { // HL
			wg.Wait()
			close(c) // HL
		}()
		// No select needed here, since errc is buffered.
		errc <- err // HL
	}()
	return c, errc
}

// Chunk seeker
type chunkSeeker struct {
	offset,
	chunkSize,
	ptr,
	remainder int64
}

func (s *chunkSeeker) Seek(offset int64, whence int) (int64, error) {

	switch whence {
	case io.SeekStart:
		s.ptr = offset
	case io.SeekCurrent:
		s.ptr = s.ptr + offset
	case io.SeekEnd:
		s.ptr = s.chunkSize + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if s.ptr < 0 {
		return 0, fmt.Errorf("negative position")
	}

	s.remainder = s.chunkSize - s.ptr
	return s.ptr, nil
}

// Chunk reader seeker
type ChunkReadSeeker struct {
	chunkSeeker

	source io.ReaderAt
}

func NewChunkReadSeeker(source io.ReaderAt, offset, chunkSize int64) io.ReadSeeker {

	return &ChunkReadSeeker{
		chunkSeeker: chunkSeeker{
			offset:    offset,
			chunkSize: chunkSize,

			ptr:       0,         // current position in chunk
			remainder: chunkSize, // unprocessed space in chunk
		},

		source: source,
	}
}

func (r *ChunkReadSeeker) Read(data []byte) (int, error) {

	var (
		err  error
		size int64
		n    int
	)

	size = int64(len(data))
	if r.remainder > size {
		n, err = r.source.ReadAt(data, r.offset+r.ptr)

		size = int64(n)
		if err != io.EOF {
			r.remainder = r.remainder - size
		} else {
			r.remainder = 0
		}
		r.ptr = r.ptr + size
		return n, err

	} else if r.remainder > 0 {
		if n, err = r.source.ReadAt(data[0:r.remainder], r.offset+r.ptr); err == nil {
			// no more bytes in chunk so return EOF
			err = io.EOF
		}

		r.remainder = 0
		r.ptr = r.ptr + int64(n)
		return n, err
	}
	return 0, io.EOF
}

// Chunk write
type ChunkWriteSeeker struct {
	chunkSeeker

	dest io.WriterAt
}

func NewChunkWriteSeeker(dest io.WriterAt, offset, chunkSize int64) io.WriteSeeker {

	return &ChunkWriteSeeker{
		chunkSeeker: chunkSeeker{
			offset:    offset,
			chunkSize: chunkSize,

			ptr:       0,         // current position in chunk
			remainder: chunkSize, // unprocessed space in chunk
		},

		dest: dest,
	}
}

func (r *ChunkWriteSeeker) Write(data []byte) (int, error) {

	var (
		err  error
		size int64
		n    int
	)

	size = int64(len(data))
	if r.remainder > size {
		n, err = r.dest.WriteAt(data, r.offset+r.ptr)

		size = int64(n)
		r.remainder = r.remainder - size
		r.ptr = r.ptr + size
		return n, err

	} else if r.remainder > 0 {
		n, err = r.dest.WriteAt(data[0:r.remainder], r.offset+r.ptr)

		r.remainder = 0
		r.ptr = r.ptr + int64(n)
		return n, err
	}
	return 0, nil
}

func (r *ChunkWriteSeeker) Seek(offset int64, whence int) (int64, error) {

	switch whence {
	case io.SeekStart:
		r.ptr = offset
	case io.SeekCurrent:
		r.ptr = r.ptr + offset
	case io.SeekEnd:
		r.ptr = r.chunkSize + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if r.ptr < 0 {
		return 0, fmt.Errorf("negative position")
	}

	r.remainder = r.chunkSize - r.ptr
	return r.ptr, nil
}
