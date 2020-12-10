package recruiter

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func register1(t *testing.T, addr string, nodename string, fp int, np int, rp int) *WorkerPool {
	wp := NewWorkerPool()
	registered, err := wp.Register(addr, nodename)
	require.NoError(t, err)
	assert.True(t, registered)
	assertPoolLen(t, wp, fp, np, rp)
	return wp
}

func assertPoolLen(t *testing.T, wp *WorkerPool, fp int, np int, rp int) {
	assert.Equal(t, fp, wp.LenFreePool(), "FreePool len=%d", fp)
	require.Equal(t, np, wp.LenNodePool(), "NodePool len=%d", np)
	assert.Equal(t, rp, wp.LenReservedPool(), "ReservedPool len=%d", rp)
}

func TestBadCalls(t *testing.T) {
	wp := NewWorkerPool()
	registered, err := wp.Register("a.b;;5000", "n1")
	assert.NotNil(t, err)
	assert.False(t, registered)
	assertPoolLen(t, wp, 0, 0, 0)
	registered, err = wp.Register("a.b:5000", "")
	assert.NotNil(t, err)
	assert.False(t, registered)
	assertPoolLen(t, wp, 0, 0, 0)
}

func TestRegisterTwice(t *testing.T) {
	wp := register1(t, "a.b:5000", "n1", 1, 1, 0)
	wp.Register("a.b:5000", "n1")
	assertPoolLen(t, wp, 1, 1, 0)
}

func TestDeregister1(t *testing.T) {
	wp := register1(t, "a.b:5000", "n2", 1, 1, 0)
	wp.Register("a.b:5001", "n2")
	wp.Deregister("a.b:5000")
	assertPoolLen(t, wp, 1, 1, 0)
}

func TestRecruit1(t *testing.T) {
	addr := "a.b:5000"
	nodename := "n2"
	wp := register1(t, addr, nodename, 1, 1, 0)
	s, err := wp.Recruit(1)
	require.NoError(t, err)
	require.Len(t, s, 1)
	assertPoolLen(t, wp, 0, 0, 1)
	assert.Equal(t, addr, s[0].Addr)
	// try recruiting 0 workers
	_, err = wp.Recruit(0)
	assert.EqualError(t, err, "recruit must request one or more workers: n=0")
	// attempt to register reserved worker should be ignored
	wp.Register(addr, nodename)
	assertPoolLen(t, wp, 0, 0, 1)
	wp.Unreserve([]string{addr})
	assertPoolLen(t, wp, 0, 0, 0)
	// recruit with none available returns empty list
	s, err = wp.Recruit(1)
	require.NoError(t, err)
	assert.Len(t, s, 0)
}

func TestRegister10(t *testing.T) {
	wp := NewWorkerPool()
	var addr string
	var nodename string
	for i := 0; i < 5; i++ {
		nodename = "n" + strconv.Itoa(i)
		for j := 0; j < 2; j++ {
			addr = nodename + ".x:" + strconv.Itoa(5000+j)
			registered, err := wp.Register(addr, nodename)
			require.NoError(t, err)
			assert.True(t, registered)
		}
	}
	assertPoolLen(t, wp, 10, 5, 0)
	s, err := wp.Recruit(5)
	require.NoError(t, err)
	assert.Len(t, s, 5)
	assertPoolLen(t, wp, 5, 5, 5)
}

func initNodesWithWorkers(t *testing.T, wp *WorkerPool, width int, height int) {
	var addr string
	var nodename string
	for i := 0; i < width; i++ {
		nodename = "n" + strconv.Itoa(i)
		for j := 0; j < height; j++ {
			addr = nodename + ".x:" + strconv.Itoa(5000+j)
			registered, err := wp.Register(addr, nodename)
			require.NoError(t, err)
			assert.True(t, registered)
		}
	}
}

func initNodesOfVaryingSize(t *testing.T, wp *WorkerPool, size int) {
	var addr string
	var nodename string
	for i := 0; i < size; i++ {
		nodename = "n" + strconv.Itoa(i)
		for j := 0; j < i+1; j++ {
			addr = nodename + ".x:" + strconv.Itoa(5000+j)
			registered, err := wp.Register(addr, nodename)
			require.NoError(t, err)
			assert.True(t, registered)
		}
	}
}

func TestRecruitFromVariablePool(t *testing.T) {
	wp := NewWorkerPool()
	initNodesOfVaryingSize(t, wp, 5)
	assertPoolLen(t, wp, 15, 5, 0)
	s, err := wp.Recruit(14)
	require.NoError(t, err)
	assert.Len(t, s, 14)
	assert.Equal(t, wp.LenFreePool(), 1)
	assert.Equal(t, wp.LenReservedPool(), 14)
}

func TestRecruitTooMany(t *testing.T) {
	wp := NewWorkerPool()
	initNodesOfVaryingSize(t, wp, 5)
	assertPoolLen(t, wp, 15, 5, 0)
	s, err := wp.Recruit(20)
	require.NoError(t, err)
	assert.Len(t, s, 15)
	assert.Equal(t, wp.LenFreePool(), 0)
	assert.Equal(t, wp.LenReservedPool(), 15)
}

func TestRecruitTwice(t *testing.T) {
	wp := NewWorkerPool()
	initNodesOfVaryingSize(t, wp, 5)
	assertPoolLen(t, wp, 15, 5, 0)
	s, err := wp.Recruit(10)
	//println(fmt.Sprintf("%v", s))
	require.NoError(t, err)
	assert.Len(t, s, 10)
	assert.Equal(t, wp.LenFreePool(), 5)
	assert.Equal(t, wp.LenReservedPool(), 10)
	s, err = wp.Recruit(7)
	require.NoError(t, err)
	assert.Len(t, s, 5)
	assert.Equal(t, wp.LenFreePool(), 0)
	assert.Equal(t, wp.LenReservedPool(), 15)
}

func TestRandomRecruitFromVariablePool(t *testing.T) {
	wp := NewWorkerPool()
	size := 20
	initNodesOfVaryingSize(t, wp, size)
	numWorkers := size * (size + 1) / 2
	assert.Equal(t, wp.LenFreePool(), numWorkers)
	assert.Equal(t, wp.LenReservedPool(), 0)
	remainingWorkers := numWorkers
	for remainingWorkers > 0 {
		numRecruits := wp.r.Intn(size) + 1
		s, err := wp.Recruit(numRecruits)
		require.NoError(t, err)
		remainingWorkers -= len(s)
		assert.Equal(t, wp.LenFreePool(), remainingWorkers)
		assert.Equal(t, wp.LenReservedPool(), numWorkers-remainingWorkers)
	}
}

func TestRandomWithReregister(t *testing.T) {
	// This test re-registers previously recruited workers
	// in a random order after each recruit call.
	// This is a rough simulation of a multi-user load.
	wp := NewWorkerPool()
	size := 30
	qsize := 30
	initNodesOfVaryingSize(t, wp, size)
	numWorkers := size * (size + 1) / 2
	assert.Equal(t, wp.LenFreePool(), numWorkers)
	assert.Equal(t, wp.LenReservedPool(), 0)
	q := make([][]WorkerDetail, qsize)
	// The Reregister Queue is initialized with empty lists, then
	// previously recruited lists of workers are shuffled in.
	for i := 0; i < qsize; i++ {
		q[i] = make([]WorkerDetail, 0)
	}
	remainingWorkers := numWorkers
	for i := 0; i < size; i++ {
		numRecruits := wp.r.Intn(2*size) + 1
		s, err := wp.Recruit(numRecruits)
		require.NoError(t, err)
		//println("iteration", i, "recruit", len(s), "from", remainingWorkers, "for", remainingWorkers-len(s))
		remainingWorkers -= len(s)
		require.Equal(t, wp.LenFreePool(), remainingWorkers)
		require.Equal(t, wp.LenReservedPool(), numWorkers-remainingWorkers)

		j := wp.r.Intn(len(q)-2) + 1
		q = append(q, s)
		copy(q[j+1:], q[j:])
		q[j] = s

		if len(q) > 2 {
			reregisterNow := q[0]
			q = q[1:]

			for _, wd := range reregisterNow {
				wp.Unreserve([]string{wd.Addr})
				wp.Register(wd.Addr, wd.NodeName)
				require.NoError(t, err)
			}
			remainingWorkers += len(reregisterNow)
			//println("iteration", i, "register", len(reregisterNow), "for", remainingWorkers)
			require.Equal(t, wp.LenFreePool(), remainingWorkers)
			require.Equal(t, wp.LenReservedPool(), numWorkers-remainingWorkers)
		}
	}
}

func TestRandomRecruitDetectSiblings(t *testing.T) {
	// This is a helpful test for tuning the algorithm
	// because we can vary the params on the next three lines
	// and toggle the value of wp.SkipSpread.
	width, height := 16, 20
	reqmin, reqmax, qsize := 4, 16, 30
	iterations := 50

	totalRecruited := 0.0
	totalSibCount := 0.0

	wp := NewWorkerPool()
	wp.SkipSpread = false
	initNodesWithWorkers(t, wp, width, height)
	numWorkers := width * height
	assert.Equal(t, wp.LenFreePool(), numWorkers)
	assert.Equal(t, wp.LenReservedPool(), 0)
	q := make([][]WorkerDetail, qsize)
	for i := 0; i < qsize; i++ {
		q[i] = make([]WorkerDetail, 0)
	}
	remainingWorkers := numWorkers
	for i := 0; i < iterations; i++ {
		numRecruits := wp.r.Intn(reqmax-reqmin) + reqmin
		s, err := wp.Recruit(numRecruits)
		require.NoError(t, err)
		//println("iteration", i, "recruit", len(s), "from", remainingWorkers, "for", remainingWorkers-len(s))
		remainingWorkers -= len(s)
		require.Equal(t, wp.LenFreePool(), remainingWorkers)
		require.Equal(t, wp.LenReservedPool(), numWorkers-remainingWorkers)

		j := wp.r.Intn(len(q)-2) + 1
		q = append(q, s)
		copy(q[j+1:], q[j:])
		q[j] = s

		if len(q) > 2 {
			reregisterNow := q[0]
			q = q[1:]

			for _, wd := range reregisterNow {
				wp.Unreserve([]string{wd.Addr})
				wp.Register(wd.Addr, wd.NodeName)
				require.NoError(t, err)
			}
			remainingWorkers += len(reregisterNow)
			//println("iteration", i, "register", len(reregisterNow), "for", remainingWorkers)
			require.Equal(t, wp.LenFreePool(), remainingWorkers)
			require.Equal(t, wp.LenReservedPool(), numWorkers-remainingWorkers)
		}

		// count siblings in recruited set
		for _, wd := range s {
			sibCount := -1 // avoid counting self
			for _, sib := range s {
				if sib.NodeName == wd.NodeName {
					sibCount++
				}
			}
			totalSibCount += float64(sibCount)
			totalRecruited++
		}
	}
	avgSiblings := totalSibCount / totalRecruited
	assert.Less(t, avgSiblings, 0.1)
	// Uncomment these for tuning:
	// println(fmt.Sprintf("SkipSpread=%v  Average number of siblings=%6.4f",
	// 	wp.SkipSpread, totalSibCount/totalRecruited))
	// require.Equal(t, 1, 0)
}
