package response

type MyCode int

const (
	SuccessResponse MyCode = 0
	ErrorResponse   MyCode = 1000
)

type Response struct {
	Code    MyCode      `json:"code"`
	Message string      `json:"message"`
	Reason  string      `json:"reason,omitempty"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

type PageData struct {
	Data     interface{} `json:"data"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

func OK(message string, data interface{}) Response {
	return Response{
		Code:    SuccessResponse,
		Message: message,
		Success: true,
		Data:    data,
	}
}

func OKWithMessage(message string) Response {
	return Response{
		Code:    SuccessResponse,
		Message: message,
		Success: true,
		Data:    nil,
	}
}

func OKWithData(data interface{}) Response {
	return Response{
		Code:    SuccessResponse,
		Message: "success",
		Success: true,
		Data:    data,
	}
}

func OKWithPageData(data interface{}, total, page, pageSize int) Response {
	return OKWithData(PageData{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func FailWithCode(message, reason string) Response {
	return Response{
		Code:    ErrorResponse,
		Message: message,
		Reason:  reason,
		Success: false,
		Data:    nil,
	}
}
