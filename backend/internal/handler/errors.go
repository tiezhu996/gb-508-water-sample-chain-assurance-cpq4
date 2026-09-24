package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/service"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput), errors.Is(err, service.ErrReviewBlocked):
		// Only surface the domain-facing cause; wrapped sentinel text is internal.
		message := err.Error()
		for _, prefix := range []string{
			service.ErrReviewBlocked.Error() + ": ",
			service.ErrInvalidInput.Error() + ": ",
			service.ErrInvalidTransition.Error() + ": ",
		} {
			message = strings.TrimPrefix(message, prefix)
		}
		if strings.Contains(message, " -> ") {
			message = "请求的状态迁移不被允许：" + strings.ReplaceAll(message, " -> ", " → ")
		}
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", message)
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
