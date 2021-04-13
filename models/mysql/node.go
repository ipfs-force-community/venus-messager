package mysql

import (
	"reflect"
	"time"

	"github.com/hunjixin/automapper"
	"gorm.io/gorm"

	"github.com/ipfs-force-community/venus-messager/models/repo"
	"github.com/ipfs-force-community/venus-messager/types"
)

type mysqlNode struct {
	ID types.UUID `gorm:"column:id;type:varchar(256);primary_key;"` // 主键

	Name  string         `gorm:"column:name;type:varchar(256);NOT NULL"`
	URL   string         `gorm:"column:url;type:varchar(256);NOT NULL"`
	Token string         `gorm:"column:token;type:varchar(256);NOT NULL"`
	Type  types.NodeType `gorm:"column:node_type;type:int"`

	IsDeleted int       `gorm:"column:is_deleted;index;default:-1;NOT NULL"` // 是否删除 1:是  -1:否
	CreatedAt time.Time `gorm:"column:created_at;index;NOT NULL"`            // 创建时间
	UpdatedAt time.Time `gorm:"column:updated_at;index;NOT NULL"`            // 更新时间
}

func FromNode(node *types.Node) *mysqlNode {
	return automapper.MustMapper(node, TMysqlNode).(*mysqlNode)
}

func (mysqlNode mysqlNode) Node() *types.Node {
	return automapper.MustMapper(&mysqlNode, TNode).(*types.Node)
}

func (mysqlNode mysqlNode) TableName() string {
	return "nodes"
}

var _ repo.NodeRepo = (*mysqlNodeRepo)(nil)

type mysqlNodeRepo struct {
	*gorm.DB
}

func newMysqlNodeRepo(db *gorm.DB) mysqlNodeRepo {
	return mysqlNodeRepo{DB: db}
}

func (s mysqlNodeRepo) CreateNode(node *types.Node) error {
	sNode := FromNode(node)
	sNode.CreatedAt = time.Now()
	sNode.UpdatedAt = time.Now()
	return s.DB.Create(sNode).Error
}

func (s mysqlNodeRepo) SaveNode(node *types.Node) error {
	sNode := FromNode(node)
	sNode.UpdatedAt = time.Now()
	return s.DB.Save(sNode).Error
}

func (s mysqlNodeRepo) GetNode(name string) (*types.Node, error) {
	var node mysqlNode
	if err := s.DB.Where("name = ? and is_deleted = -1", name).First(&node).Error; err != nil {
		return nil, err
	}
	return node.Node(), nil
}

func (s mysqlNodeRepo) HasNode(name string) (bool, error) {
	var count int64
	if err := s.DB.Model(&mysqlNode{}).Where("name = ? and is_deleted = -1", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s mysqlNodeRepo) ListNode() ([]*types.Node, error) {
	var internalNode []*mysqlNode
	if err := s.DB.Find(&internalNode, "is_deleted = ?", -1).Error; err != nil {
		return nil, err
	}

	result, err := automapper.Mapper(internalNode, reflect.TypeOf([]*types.Node{}))
	if err != nil {
		return nil, err
	}
	return result.([]*types.Node), nil
}

func (s mysqlNodeRepo) DelNode(name string) error {
	var node mysqlNode
	if err := s.DB.Where("name = ? and is_deleted = -1", name).First(&node).Error; err != nil {
		return err
	}
	node.IsDeleted = 1

	return s.DB.Save(&node).Error
}
