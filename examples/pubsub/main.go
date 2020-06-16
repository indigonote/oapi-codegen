package main

import (
	"log"
	"regexp"
	"time"

	"gopkg.in/go-playground/validator.v9"
	"sios.tech/indigo/oapi-codegen/examples/pubsub/message"
	openapi_types "sios.tech/indigo/oapi-codegen/pkg/types"
)

func main() {
	validate := validator.New()
	validate.RegisterValidation("regex", Regexp)

	user := message.User{
		Id:   "userId",
		Name: "name",
	}
	if err := validate.Struct(&user); err != nil {
		log.Println(err)
	}
	mp := message.MedicalPoint{
		AnnouncementDate: openapi_types.Date{Time: time.Now()},
		EffectiveDate:    openapi_types.Date{Time: time.Now()},
		Id:               "10000",
		Point:            10,
		Segment:          "xxx",
	}
	if err := validate.Struct(&mp); err != nil {
		log.Println(err)
	}

}

func Regexp(fl validator.FieldLevel) bool {
	re := regexp.MustCompile(fl.Param())
	return re.MatchString(fl.Field().String())
}
