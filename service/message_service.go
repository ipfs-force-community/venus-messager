package service

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	venusTypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-messager/config"
	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

const MaxHeadChangeProcess = 5

const LookBackLimit = 1000

type MessageService struct {
	repo       repo.Repo
	log        *logrus.Logger
	cfg        *config.MessageServiceConfig
	nodeClient *NodeClient

	headChans chan *headChan

	messageState *MessageState
	tsCache      map[uint64]*tipsetFormat

	l sync.Mutex
}

type headChan struct {
	apply, revert []*venusTypes.TipSet
}

func NewMessageService(repo repo.Repo, nc *NodeClient, logger *logrus.Logger, cfg *config.MessageServiceConfig, messageState *MessageState) (*MessageService, error) {
	ms := &MessageService{
		repo:         repo,
		log:          logger,
		nodeClient:   nc,
		cfg:          cfg,
		headChans:    make(chan *headChan, MaxHeadChangeProcess),
		messageState: messageState,
		tsCache:      make(map[uint64]*tipsetFormat),
	}
	ms.checkFile()
	ms.refreshMessageState(context.TODO())

	return ms, nil
}

func (ms *MessageService) checkFile() error {
	if _, err := os.Stat(ms.cfg.TipsetFilePath); err != nil {
		if os.IsNotExist(err) {
			if _, err := os.Create(ms.cfg.TipsetFilePath); err != nil {
				return err
			}
		}
		return err
	}

	return nil
}

func (ms *MessageService) PushMessage(ctx context.Context, msg *types.Message) (string, error) {
	msg.State = types.Unsigned
	ms.messageState.SetMessage(msg)
	return ms.repo.MessageRepo().SaveMessage(msg)
}

func (ms *MessageService) GetMessage(ctx context.Context, uuid string) (*types.Message, error) {
	if msg, ok := ms.messageState.GetMessage(uuid); ok {
		return msg, nil
	}
	return ms.repo.MessageRepo().GetMessage(uuid)
}

func (ms *MessageService) GetMessageByCid(background context.Context, cid string) (*types.Message, error) {
	return ms.repo.MessageRepo().GetMessageByCid(cid)
}

func (ms *MessageService) ListMessage(ctx context.Context) ([]*types.Message, error) {
	return ms.repo.MessageRepo().ListMessage()
}

func (ms *MessageService) ProcessNewHead(ctx context.Context, apply, revert []*venusTypes.TipSet) error {
	ms.log.Infof("receive new head from chain")
	ms.headChans <- &headChan{
		apply:  apply,
		revert: revert,
	}
	return nil
}

func (ms *MessageService) ReconnectCheck(ctx context.Context, head *venusTypes.TipSet) error {
	ms.log.Infof("reconnect to node")
	now := time.Now()
	tsList, err := readTipsetFromFile(ms.cfg.TipsetFilePath)
	ms.log.Infof("read tipset file cost: %v 's'", time.Since(now).Seconds())
	if err != nil {
		return xerrors.Errorf("read tipset info failed %v", err)
	}

	if len(tsList) == 0 {
		return nil
	}

	sort.Sort(tsList)
	ms.tsCache = tsList.Map()

	if tsList[0].Height == uint64(head.Height()) && isEqual(tsList[0], head) {
		ms.log.Infof("The head does not change and returns directly.")
		return nil
	}

	gapTipset, idx, err := ms.lookAncestors(ctx, tsList, head)
	if err != nil {
		return err
	}

	if len(gapTipset) == 0 {
		return nil
	}

	// handle revert
	if tsList[0].Height > uint64(head.Height()) || (tsList[0].Height == uint64(head.Height()) && !isEqual(tsList[0], head)) {
		if idx > len(tsList) {
			if err := os.Rename(ms.cfg.TipsetFilePath, ms.cfg.TipsetFilePath+".old"); err != nil {
				return err
			}
		} else {
			updateTipsetFile(ms.cfg.TipsetFilePath, tsList[idx:])
		}
	}

	err = ms.doRefreshMessageState(ctx, &headChan{
		apply:  gapTipset,
		revert: nil,
	})

	return err
}

func (ms *MessageService) lookAncestors(ctx context.Context, localTipset tipsetList, head *venusTypes.TipSet) ([]*venusTypes.TipSet, int, error) {
	var err error

	ts := &venusTypes.TipSet{}
	*ts = *head

	localTs := localTipset[0]
	idx := 0
	localTsLen := len(localTipset)

	gapTipset := make([]*venusTypes.TipSet, 0, 0)
	loopCount := 0
	for {
		if loopCount > LookBackLimit {
			break
		}
		if idx >= localTsLen {
			break
		}
		if ts.Height() == 0 {
			break
		}
		if localTs.Height > uint64(ts.Height()) {
			idx++
		} else if localTs.Height == uint64(ts.Height()) {
			if isEqual(localTs, ts) {
				break
			}
			idx++
		} else {
			gapTipset = append(gapTipset, ts)
			ts, err = ms.nodeClient.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				return nil, 0, xerrors.Errorf("get tipset failed %v", err)
			}
		}
		loopCount++
	}

	return gapTipset, idx, nil
}

func isEqual(tf *tipsetFormat, ts *venusTypes.TipSet) bool {
	if tf.Height != uint64(ts.Height()) {
		return false
	}
	if len(tf.Cid) != len(ts.Cids()) {
		return false
	}
	cidMap := make(map[string]struct{}, len(tf.Cid))
	for _, cid := range tf.Cid {
		cidMap[cid] = struct{}{}
	}
	for _, block := range ts.Cids() {
		if _, ok := cidMap[block.String()]; !ok {
			return false
		}
	}
	return true
}
