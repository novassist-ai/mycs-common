package utils_test

import (
	"fmt"
	"sync"

	"github.com/novassist/mycs-common/pkg/goutils/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("task utils tests", func() {

	It("queues 2 tasks with 0 retries", func() {

		td := utils.NewTaskDispatcher(1, 1000)
		td.Start(1)

		runWG := &sync.WaitGroup{}
		runWG.Add(2)

		testTask1 := &testTask{
			numFails: 0,
			data: []int{ 1, 2, 3, 4, 5 },
			expectData: []int{ 1, 2, 3, 4, 5 },
			runWG: runWG,
		}
		testTask2 := &testTask{
			data: []int{ 6,7,8,9 },
			numFails: 0,
			expectData: []int{ 6,7,8,9 },
			runWG: runWG,
		}

		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask1", testTask1.task).
				WithData(testTask1.data).
				OnSuccess(testTask1.success).
				OnError(testTask1.error).Once()
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask2", testTask2.task).
				WithData(testTask2.data).
				OnSuccess(testTask2.success).
				OnError(testTask2.error).Once()
			Expect(err).NotTo(HaveOccurred())
		}()
	
		runWG.Wait()
		td.Stop()

		Expect(testTask1.succeeded).To(BeTrue())
		Expect(testTask1.failed).To(BeFalse())
		Expect(testTask2.succeeded).To(BeTrue())
		Expect(testTask2.failed).To(BeFalse())
	})

	It("queues 2 tasks and 2nd times out as 1st holds the dispatch queue as there is only 1 worker", func() {

		td := utils.NewTaskDispatcher(1, 1000)
		td.Start(1)

		runWG := &sync.WaitGroup{}
		runWG.Add(2)

		testTask1 := &testTask{
			numFails: 4,
			data: []int{ 1, 2, 3, 4, 5 },
			expectData: []int{ 5 },
			runWG: runWG,
		}
		testTask2 := &testTask{
			data: []int{ 6, 7, 8, 9 },
			numFails: 2,
			expectData: []int{ 7, 8, 9 },
			runWG: runWG,
		}

		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask1", testTask1.task).
				WithData(testTask1.data).
				OnSuccess(testTask1.success).
				OnError(testTask1.error).
				WithRetries(3)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask2", testTask2.task).
				WithData(testTask2.data).
				OnSuccess(testTask2.success).
				OnError(testTask2.error).
				WithRetries(3)
			Expect(err).NotTo(HaveOccurred())
		}()
	
		runWG.Wait()
		td.Stop()

		Expect(testTask1.succeeded).To(BeFalse())
		Expect(testTask1.failed).To(BeTrue())
		Expect(testTask2.succeeded).To(BeFalse())
		Expect(testTask2.failed).To(BeTrue())
	})

	It("queues 2 tasks with 3 retries, 1 succeeds after 2 retries the other fails", func() {

		td := utils.NewTaskDispatcher(1, 1000)
		td.Start(2)

		runWG := &sync.WaitGroup{}
		runWG.Add(2)

		testTask1 := &testTask{
			numFails: 4,
			data: []int{ 1, 2, 3, 4, 5 },
			expectData: []int{ 5 },
			runWG: runWG,
		}
		testTask2 := &testTask{
			data: []int{ 6, 7, 8, 9 },
			numFails: 2,
			expectData: []int{ 8, 9 },
			runWG: runWG,
		}

		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask1", testTask1.task).
				WithData(testTask1.data).
				OnSuccess(testTask1.success).
				OnError(testTask1.error).
				WithRetries(3)
			Expect(err).NotTo(HaveOccurred())
		}()
		go func() {
			defer GinkgoRecover()
			err := td.RunTask("testTask2", testTask2.task).
				WithData(testTask2.data).
				OnSuccess(testTask2.success).
				OnError(testTask2.error).
				WithRetries(3)
			Expect(err).NotTo(HaveOccurred())
		}()
	
		runWG.Wait()
		td.Stop()

		Expect(testTask1.succeeded).To(BeFalse())
		Expect(testTask1.failed).To(BeTrue())
		Expect(testTask2.succeeded).To(BeTrue())
		Expect(testTask2.failed).To(BeFalse())
	})
})

type testTask struct {
	numFails int

	data       []int
	expectData []int

	runWG *sync.WaitGroup
	
	succeeded, failed bool
}

func (t *testTask) task(inData interface{}) (outData interface{}, err error) {

	if t.numFails == 0 {
		return t.data, nil
	}
	t.numFails--
	t.data = t.data[1:]
	return t.data, fmt.Errorf("failed task")
}

func (t *testTask) success(outData interface{}) {
	defer GinkgoRecover()
	defer t.runWG.Done()

	Expect(outData).To(Equal(t.expectData))
	Expect(t.numFails).To(Equal(0))
	t.succeeded = true
}

func (t *testTask) error(err error, outData interface{}) {
	defer GinkgoRecover()
	defer t.runWG.Done()

	Expect(err.Error()).To(Equal("failed task"))
	Expect(outData).To(Equal(t.expectData))
	Expect(t.numFails).To(BeNumerically(">=", 0))
	t.failed = true
}
