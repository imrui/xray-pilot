package repository

import "errors"

var ErrNodeKeyLocked = errors.New("该节点协议已锁定，请先解锁后再修改")
