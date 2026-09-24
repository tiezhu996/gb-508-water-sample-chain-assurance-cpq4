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
	case errors.Is(err, service.ErrReviewBlocked):
		reason := strings.TrimPrefix(err.Error(), service.ErrReviewBlocked.Error()+": ")
		if !strings.HasPrefix(reason, "复核单已被拦截留档") {
			reason = "签发被拦截: " + reason
		}
		util.Fail(c, http.StatusUnprocessableEntity, "review_blocked", reason)
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
