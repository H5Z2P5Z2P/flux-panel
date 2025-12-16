package dto

type StatisticsFlowDto struct {
	ID          *int64 `json:"id"`
	UserId      int64  `json:"userId"`
	Flow        int64  `json:"flow"`
	TotalFlow   int64  `json:"totalFlow"`
	Time        string `json:"time"`
	CreatedTime *int64 `json:"createdTime"`
}
