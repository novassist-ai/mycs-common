package utils_test

import (
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const fileSize = 10240

var _ = Describe("file utils tests", func() {

	Context("reading a file in chunks", func() {

		var (
			err error

			tmpfile1,
			tmpfile2 *os.File
			content string
		)

		BeforeEach(func() {
			rand.Seed(time.Now().UTC().UnixNano())

			tmpfile1, err = ioutil.TempFile("", "chunkreadtest")
			defer func() { _ = tmpfile1.Close() }()
			Expect(err).ToNot(HaveOccurred())

			tmpfile2, err = ioutil.TempFile("", "chunkwritetest")
			defer func() { _ = tmpfile2.Close() }()
			Expect(err).ToNot(HaveOccurred())

			content = utils.RandomString(fileSize)
			_, err = tmpfile1.Write([]byte(content))
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.Remove(tmpfile1.Name())
			os.Remove(tmpfile2.Name())
		})

		It("read file in chunks", func() {

			var (
				file     *os.File
				fileInfo os.FileInfo
				fileData strings.Builder

				wg sync.WaitGroup
			)

			// pick a chunk size between 1 and 99
			chunkSize := rand.Intn(99) + 1

			logger.DebugMessage("Reading file %s in chunks", tmpfile1.Name())
			file, err = os.Open(tmpfile1.Name())
			Expect(err).ToNot(HaveOccurred())
			defer file.Close()

			fileInfo, err := file.Stat()
			Expect(err).ToNot(HaveOccurred())
			size := int(fileInfo.Size())
			logger.DebugMessage("Reading file of size %d in chunks", size)

			numChunks := size / chunkSize
			lastPartialChunkSize := size % chunkSize
			if lastPartialChunkSize > 0 {
				numChunks++
			}
			chunkedData := make([]strings.Builder, numChunks)
			logger.DebugMessage("Reading %d chunks of file each of size %d", numChunks, chunkSize)

			wg.Add(numChunks)
			for i := 0; i < numChunks; i++ {

				// pick a buffer size which is a fraction of the chunk size to read a chunk
				bufferSize := rand.Intn(chunkSize) + 1
				logger.DebugMessage("Reading chunk %d using a buffer with size %d", i, bufferSize)

				go func(chunkIndex, bufferSize int) {
					defer wg.Done()
					defer GinkgoRecover()

					reader := utils.NewChunkReadSeeker(file, int64(chunkIndex*chunkSize), int64(chunkSize))
					buffer := make([]byte, bufferSize)

					// seek to beginning and end of chunk
					// to exercise Seeker interface
					p, err := reader.Seek(0, io.SeekEnd)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(int64(chunkSize)))
					_, err = reader.Seek(int64(rand.Intn(chunkSize)), io.SeekCurrent)
					Expect(err).ToNot(HaveOccurred())
					p, err = reader.Seek(0, io.SeekStart)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(int64(0)))

					logger.DebugMessage("Reading chunk %d", chunkIndex)
					for {
						n, err := reader.Read(buffer)
						chunkedData[chunkIndex].Write(buffer[0:n])
						if err != nil {
							if err == io.EOF {
								break
							}
							Expect(err).ToNot(HaveOccurred())
						}
					}
					logger.DebugMessage("Read chunk %d: %s", chunkIndex, chunkedData[chunkIndex].String())
				}(i, bufferSize)
			}
			wg.Wait()

			// compose file data and validate chunk sizes
			for i := 0; i < numChunks; i++ {
				n, err := fileData.WriteString(chunkedData[i].String())
				Expect(err).ToNot(HaveOccurred())
				if i == numChunks-1 && lastPartialChunkSize > 0 {
					Expect(n).To(Equal(lastPartialChunkSize))
				} else {
					Expect(n).To(Equal(chunkSize))
				}
			}
			Expect(fileData.String()).To(Equal(content))
		})

		It("write file in chunks", func() {

			var (
				file     *os.File
				fileInfo os.FileInfo
				fileData strings.Builder

				wg sync.WaitGroup
			)

			// pick a chunk size between 1 and 99
			chunkSize := rand.Intn(99) + 1

			logger.DebugMessage("Writing file %s in chunks", tmpfile2.Name())
			file, err = os.OpenFile(tmpfile2.Name(), os.O_RDWR, 0644)
			Expect(err).ToNot(HaveOccurred())
			defer file.Close()

			size := len(content)
			logger.DebugMessage("Writing data of size %d in chunks", size)

			numChunks := size / chunkSize
			lastPartialChunkSize := size % chunkSize
			if lastPartialChunkSize > 0 {
				numChunks++
			}
			logger.DebugMessage("Writing %d chunks of data each of size %d", numChunks, chunkSize)

			wg.Add(numChunks)
			for i := 0; i < numChunks; i++ {

				// pick a buffer size which is a fraction of the chunk size to write a chunk data
				bufferSize := rand.Intn(chunkSize) + 1
				logger.DebugMessage("Writing chunk %d, %d chars at a time", i, bufferSize)

				go func(chunkIndex, bufferSize int) {
					defer wg.Done()
					defer GinkgoRecover()

					writer := utils.NewChunkWriteSeeker(file, int64(chunkIndex*chunkSize), int64(chunkSize))

					// seek to beginning and end of chunk
					// to exercise Seeker interface
					p, err := writer.Seek(0, io.SeekEnd)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(int64(chunkSize)))
					_, err = writer.Seek(int64(rand.Intn(chunkSize)), io.SeekCurrent)
					Expect(err).ToNot(HaveOccurred())
					p, err = writer.Seek(0, io.SeekStart)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(int64(0)))

					offset := chunkIndex * chunkSize
					end := (chunkIndex + 1) * chunkSize
					if end > size {
						end = size
					}

					logger.DebugMessage("Writing chunk %d: %s", chunkIndex, content[offset:end])
					for offset < end {

						advance := offset + bufferSize
						if advance > end {
							advance = end
						}
						n, err := writer.Write([]byte(content[offset:advance]))
						offset = offset + n
						Expect(err).ToNot(HaveOccurred())
					}
					logger.DebugMessage("Chunk %d written", chunkIndex)
				}(i, bufferSize)
			}
			wg.Wait()
			file.Close()

			// read written file and validate the content
			file, err = os.Open(tmpfile2.Name())
			Expect(err).ToNot(HaveOccurred())

			fileInfo, err := file.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(int(fileInfo.Size())).To(Equal(size))

			_, err = io.Copy(&fileData, file)
			Expect(err).ToNot(HaveOccurred())
			Expect(fileData.String()).To(Equal(content))
		})
	})

	Context("directory compare", func() {

		It("compares ", func() {
			_, filename, _, _ := runtime.Caller(0)
			dirPath := filepath.Dir(path.Dir(filename))

			dirMatch, err := utils.DirCompare(dirPath, dirPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(dirMatch).To(BeTrue())

			dirMatch, err = utils.DirCompare(dirPath, path.Dir(filename))
			Expect(err).ToNot(HaveOccurred())
			Expect(dirMatch).To(BeFalse())
		})
	})
})
