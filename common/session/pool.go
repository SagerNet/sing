package session

import (
	"container/list"
	"sync"

	"sing/common"
)

var (
	connectionPool        list.List
	connectionPoolEnabled bool
	connectionAccess      sync.Mutex
)

func EnableConnectionPool() {
	connectionPoolEnabled = true
}

func DisableConnectionPool() {
	connectionAccess.Lock()
	defer connectionAccess.Unlock()
	connectionPoolEnabled = false
	clearConnections()
}

func AddConnection(connection any) any {
	if !connectionPoolEnabled {
		return connection
	}
	connectionAccess.Lock()
	defer connectionAccess.Unlock()
	return connectionPool.PushBack(connection)
}

func RemoveConnection(anyElement any) {
	element, ok := anyElement.(*list.Element)
	if !ok {
		common.Close(anyElement)
		return
	}
	if element.Value == nil {
		return
	}
	common.Close(element.Value)
	element.Value = nil
	connectionAccess.Lock()
	defer connectionAccess.Unlock()
	connectionPool.Remove(element)
}

func ResetConnections() {
	connectionAccess.Lock()
	defer connectionAccess.Unlock()
	clearConnections()
}

func clearConnections() {
	for element := connectionPool.Front(); element != nil; element = element.Next() {
		common.Close(element)
	}
	connectionPool.Init()
}
