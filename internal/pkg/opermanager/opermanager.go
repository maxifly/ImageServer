package opermanager

type OperMngr struct {
}

//TODO Всё сделать

type OperStatus struct {
	Status string
	Error  string
}

func NewOperMngr() *OperMngr {
	operMng := OperMngr{}
	return &operMng
}

func (op *OperMngr) StartOperation() (string, error) {
	return "q123", nil
}

func (op *OperMngr) GetOperationStatus(id string) (*OperStatus, error) {
	return &OperStatus{Status: "processing"}, nil
}

func (op *OperMngr) GetFileName(id string) (string, error) {
	return "testName.jpg", nil
}
