package dto

type ResetFlowDto struct {
	ID   int64 `json:"id" binding:"required"`
	Type int   `json:"type" binding:"required"`
}
