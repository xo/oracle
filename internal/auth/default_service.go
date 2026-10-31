package auth

import (
	"errors"
	"fmt"
	"strings"
)

type negoService interface {
	getServiceDataLength() int
	writeServiceData() error
	readServiceData(subPacketNum int) error
	validateResponse() error
	getVersion() uint32
	activateAlgorithm() error
	isNewVersion() bool
}

type defaultService struct {
	ano *AdvNego
	//comm                  *AdvancedNegoComm
	serviceType           int
	level                 int
	availableServiceNames []string
	availableServiceIDs   []int
	selectedIndices       []int
	version               uint32
	// selectedServ map[string]int
	// avaServs     map[string]int
}

func (serv *defaultService) isNewVersion() bool {
	return isNew(serv.version)
}
func (serv *defaultService) getVersion() uint32 {
	return serv.version
}

func (serv *defaultService) activateAlgorithm() error {
	return nil
}

func (serv *defaultService) writeHeader(serviceSubPackets int) {
	comm := serv.ano.comm
	comm.session.PutInt(serv.serviceType, 2, true, false)
	comm.session.PutInt(serviceSubPackets, 2, true, false)
	comm.session.PutInt(0, 4, true, false)
}

func (serv *defaultService) readAdvNegoLevel(level string) {
	level = strings.ToUpper(level)
	if level == "" || level == "ACCEPTED" {
		serv.level = 0
	} else if level == "REJECTED" {
		serv.level = 1
	} else if level == "REQUESTED" {
		serv.level = 2
	} else if level == "REQUIRED" {
		serv.level = 3
	} else {
		serv.level = -1
	}
}

func (serv *defaultService) buildServiceList(userList []string, useLevel, useDefault bool) error {
	serv.selectedIndices = make([]int, 0, 10)
	if useLevel {
		if serv.level == 1 {
			serv.selectedIndices = append(serv.selectedIndices, 0)
			return nil
		}
		if serv.level != 0 && serv.level != 2 && serv.level != 3 {
			return errors.New(fmt.Sprintf("unsupported service level value: %d", serv.level))
		}
	}
	userListLength := len(userList)
	for i := range userListLength {
		userList[i] = strings.TrimSpace(userList[i])
	}
	if userListLength > 0 && userList[userListLength-1] == "" {
		userList = userList[:userListLength-1]
	}
	if len(userList) == 0 {
		if useDefault {
			for i := 0; i < len(serv.availableServiceNames); i++ {
				if serv.availableServiceNames[i] == "" {
					if !(useLevel && serv.level == 0) {
						continue
					}
				}
				serv.selectedIndices = append(serv.selectedIndices, i)
			}
			if useLevel && serv.level == 2 {
				serv.selectedIndices = append(serv.selectedIndices, 0)
			}
		}
		return nil
	} else if len(userList) == 1 {
		if strings.ToUpper(userList[0]) == "ALL" {
			for i := 0; i < len(serv.availableServiceNames); i++ {
				if serv.availableServiceNames[i] == "" {
					if !(useLevel && serv.level == 0) {
						continue
					}
				}
				serv.selectedIndices = append(serv.selectedIndices, i)
			}
			if useLevel && serv.level == 2 {
				serv.selectedIndices = append(serv.selectedIndices, 0)
			}
			return nil
		} else if strings.ToUpper(userList[0]) == "NONE" {
			return nil
		}
	}
	if useLevel && serv.level == 0 {
		serv.selectedIndices = append(serv.selectedIndices, 0)
	}
	for _, userItem := range userList {
		if userItem == "" {
			return errors.New("empty authentication service")
		}
		found := false
		for i := 0; i < len(serv.availableServiceNames); i++ {
			if strings.ToUpper(userItem) == serv.availableServiceNames[i] {
				serv.selectedIndices = append(serv.selectedIndices, i)
				found = true
				break
			}
		}
		if !found {
			return errors.New("unsupported authentication service")
		}
	}
	if useLevel && serv.level == 2 {
		serv.selectedIndices = append(serv.selectedIndices, 0)
	}
	return nil
}

func (serv *defaultService) validateResponse() error {
	return nil
}
