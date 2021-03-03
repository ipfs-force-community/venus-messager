package mysql

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/hunjixin/automapper"

	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

type mysqlMessage struct {
	Id      string `gorm:"column:id;primary_key;" json:"id"` // 主键
	Version uint64 `gorm:"column:version;" json:"version"`

	To    string `gorm:"column:to;type:varchar(256);NOT NULL" json:"to"`
	From  string `gorm:"column:from;type:varchar(256);NOT NULL" json:"from"`
	Nonce uint64 `gorm:"column:nonce;" json:"nonce"`

	Value *types.Int `gorm:"column:value;type:varchar(256);" json:"value"`

	GasLimit   int64      `gorm:"column:gaslimit;" json:"gasLimit"`
	GasFeeCap  *types.Int `gorm:"column:gasfeecap;type:varchar(256);" json:"gasFeeCap"`
	GasPremium *types.Int `gorm:"column:gaspremium;type:varchar(256);" json:"gasPremium"`

	Method int `gorm:"column:method;" json:"method"`

	Params   []byte `gorm:"column:params;type:text;" json:"params"`
	SignData []byte `gorm:"column:signdata;type:varchar(256);" json:"signData"`

	IsDeleted int       `gorm:"column:is_deleted;default:-1;NOT NULL" json:"isDeleted"`               // 是否删除 1:是  -1:否
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;NOT NULL" json:"createAt"` // 创建时间
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP;NOT NULL" json:"updateAt"` // 更新时间

	SendSpec []byte `gorm:"column:sendspec;type:text" json:"sendSpec"`
}

func FromMessage(msg types.Message) *mysqlMessage {
	return automapper.MustMapper(&msg, TMysqlMessage).(*mysqlMessage)
}

func (m mysqlMessage) Message() *types.Message {
	return automapper.MustMapper(&m, TMessage).(*types.Message)
}

func (m *mysqlMessage) TableName() string {
	return "messages"
}

var _ repo.MessageRepo = (*mysqlMessageRepo)(nil)

type mysqlMessageRepo struct {
	repo.Repo
}

func newMysqlMessageRepo(repo repo.Repo) mysqlMessageRepo {
	return mysqlMessageRepo{repo}
}

func (m mysqlMessageRepo) SaveMessage(msg *types.Message, spec *types.SendSpec) (string, error) {
	b := []byte{}
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	msg.SendSpec = b
	err = m.GetDb().Save(msg).Error
	return msg.Id, err
}

func (m mysqlMessageRepo) GetMessage(uuid string) (*types.Message, error) {
	var msg *mysqlMessage
	if err := m.GetDb().Where(&mysqlMessage{Id: uuid, IsDeleted: -1}).First(&msg).Error; err != nil {
		return nil, err
	}
	return msg.Message(), nil
}

func (m mysqlMessageRepo) ListMessage() ([]*types.Message, error) {
	var internalMsg []*mysqlMessage
	if err := m.GetDb().Find(&internalMsg, "is_deleted = ?", -1).Error; err != nil {
		return nil, err
	}

	result, err := automapper.Mapper(internalMsg, reflect.TypeOf([]*types.Message{}))
	if err != nil {
		return nil, err
	}
	return result.([]*types.Message), nil
}
