package stats

import (
	//"github.com/cryptonote-social/csminer/crylog"

	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mutex sync.RWMutex

	// client side stats
	startTime                      time.Time // when the miner started up
	lastResetTime                  time.Time
	lastUpdateTime                 time.Time
	sharesAccepted                 int64
	sharesRejected                 int64
	poolSideHashes                 int64
	clientSideHashes, recentHashes int64

	// pool stats
	lastPoolUsername        string
	lastPoolUpdateTime      time.Time
	ppropProgress           float64
	hashrate1, hashrate24   int64
	lifetimeHashes          int64
	paid, owed, accumulated float64
	timeToReward            string
)

func Init() {
	mutex.Lock()
	defer mutex.Unlock()
	now := time.Now()
	startTime = now
	lastResetTime = now
	lastUpdateTime = now
}

// Call whenever we're at at a point where recent hashrate calculation would be accurate,
// e.g. after all worker threads have been tallied.
func RecentStatsNowAccurate() {
	lastUpdateTime = time.Now()
}

func TallyHashes(hashes int64) {
	mutex.Lock()
	defer mutex.Unlock()
	clientSideHashes += hashes
	recentHashes += hashes
}

func ShareAccepted(diffTarget int64) {
	mutex.Lock()
	defer mutex.Unlock()
	sharesAccepted++
	poolSideHashes += diffTarget
}

func ShareRejected() {
	mutex.Lock()
	defer mutex.Unlock()
	sharesRejected++
}

// Call every time an event happens that may induce a big change in hashrate,
// e.g. reseeding, adding/removing threads, restablishing a connection.
func ResetRecent() {
	mutex.Lock()
	defer mutex.Unlock()
	recentHashes = 0
	now := time.Now()
	lastUpdateTime = now
	lastResetTime = now
}

type Snapshot struct {
	SharesAccepted, SharesRejected   int64
	ClientSideHashes, PoolSideHashes int64
	// A negative value for hashrate is used to indicate "still calculating" (e.g. not enough of a
	// time window to be accurate)
	Hashrate, RecentHashrate float64

	// Pool stats
	PoolUsername            string
	LifetimeHashes          int64
	Paid, Owed, Accumulated float64
	TimeToReward            string
	SecondsOld              int // how many seconds out of date the pool stats are
}

func GetSnapshot(isMining bool) (s *Snapshot, secondsSinceReset float64, secondsRecentWindow float64) {
	mutex.Lock()
	defer mutex.Unlock()
	r := &Snapshot{}
	r.SharesAccepted = sharesAccepted
	r.SharesRejected = sharesRejected
	r.ClientSideHashes = clientSideHashes
	r.PoolSideHashes = poolSideHashes

	var elapsedOverall float64
	if isMining {
		// if we're actively mining then hash count is only accurate
		// as of the last update time
		elapsedOverall = lastUpdateTime.Sub(startTime).Seconds()
	} else {
		elapsedOverall = time.Now().Sub(startTime).Seconds()
	}
	// Recent stats are only accurate up to the last update time
	elapsedRecent := lastUpdateTime.Sub(lastResetTime).Seconds()

	if !isMining {
		r.RecentHashrate = 0.0
		if elapsedOverall > 0.0 {
			r.Hashrate = float64(clientSideHashes) / elapsedOverall
		}
	} else {
		if elapsedRecent > 5.0 {
			// For accurate results, we require at least 5 seconds of mining during the recent
			// period in order to return a recent hashrate.
			r.RecentHashrate = float64(recentHashes) / elapsedRecent
		} else {
			r.RecentHashrate = -1.0 // indicates not enough data
		}
		if elapsedOverall > 0.0 {
			r.Hashrate = float64(clientSideHashes) / elapsedOverall
		} else {
			r.Hashrate = 0.0
		}
	}

	if lastPoolUsername != "" {
		r.PoolUsername = lastPoolUsername
		r.LifetimeHashes = lifetimeHashes
		r.Paid = paid
		r.Owed = owed
		r.Accumulated = accumulated
		r.TimeToReward = timeToReward
	}
	r.SecondsOld = int(time.Now().Sub(lastPoolUpdateTime).Seconds())
	return r, time.Now().Sub(lastResetTime).Seconds(), elapsedRecent
}

func RefreshPoolStats(username string) error {
	c := &http.Client{
		Timeout: 15 * time.Second,
	}
	uri := "https://cryptonote.social/json/WorkerStats"
	sbody := "{\"Coin\": \"xmr\", \"Worker\": \"" + username + "\"}\n"
	body := strings.NewReader(sbody)
	resp, err := c.Post(uri, "", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	s := &struct {
		Code             int
		CycleProgress    float64
		Hashrate1        int64
		Hashrate24       int64
		LifetimeHashes   int64
		LifetimeBestHash int64
		Donate           float64
		AmountPaid       float64
		AmountOwed       float64
	}{}
	err = json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	// Now get pool stats
	uri = "https://cryptonote.social/json/PoolStats"
	sbody = "{\"Coin\": \"xmr\"}\n"
	body = strings.NewReader(sbody)
	resp2, err := http.DefaultClient.Post(uri, "", body)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	b, err = ioutil.ReadAll(resp2.Body)
	if err != nil {
		return err
	}
	ps := &struct {
		Code               int
		NextBlockReward    float64
		Margin             float64
		PPROPProgress      float64
		PPROPHashrate      int64
		NetworkDifficulty  int64
		SmoothedDifficulty int64 // Network difficulty averaged over the past hour
	}{}
	err = json.Unmarshal(b, &ps)
	if err != nil {
		return err
	}

	diff := float64(ps.SmoothedDifficulty)
	if diff == 0.0 {
		diff = float64(ps.NetworkDifficulty)
	}
	hr := float64(ps.PPROPHashrate)
	var ttreward string
	if hr > 0.0 {
		ttr := (diff*(1.0+ps.Margin) - (ps.PPROPProgress * diff)) / hr / 3600.0 / 24.0
		if ttr > 0.0 {
			if ttr < 1.0 {
				ttr *= 24.0
				if ttr < 1.0 {
					ttr *= 60.0
					ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " min"
				} else {
					ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " hrs"
				}
			} else {
				ttreward = strconv.FormatFloat(ttr, 'f', 2, 64) + " days"
			}
		} else if ttr < 0.0 {
			ttreward = "overdue"
		}
	}

	mutex.Lock()
	lastPoolUsername = username
	lastPoolUpdateTime = time.Now()
	hashrate1 = s.Hashrate1
	hashrate24 = s.Hashrate24
	lifetimeHashes = s.LifetimeHashes
	paid = s.AmountPaid
	owed = s.AmountOwed
	if ps.NextBlockReward > 0.0 && s.CycleProgress > 0.0 {
		progress := s.CycleProgress / (1.0 + ps.Margin)
		accumulated = ps.NextBlockReward * progress
	}
	timeToReward = ttreward
	mutex.Unlock()

	return nil
}
