package aerrzerolog

import (
	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
)

func init() {
	// Configure zerolog to use aerr's JSON marshaling
	zerolog.ErrorMarshalFunc = AerrMarshalFunc
	zerolog.ErrorStackMarshaler = AerrStackMarshaler
}

// AerrMarshalFunc returns a marshaller function that converts aerr errors
// into their JSON representation for zerolog logging.
//
// This function is automatically set in init() so you can use standard zerolog API:
//
//	logger.Error().Stack().Err(err).Msg("request failed")
func AerrMarshalFunc(err error) interface{} {
	if typedErr, ok := aerr.AsAerr(err); ok {
		return zerologErrorMarshaller{err: &typedErr}
	}
	return err
}

// AerrStackMarshaler returns a marshaller function that extracts stack trace
// information from aerr errors for zerolog logging.
//
// This function is automatically set in init() to work with zerolog's Stack() method.
func AerrStackMarshaler(err error) interface{} {
	if aErr, ok := aerr.AsAerr(err); ok {
		if stack := aErr.Traces(); len(stack) > 0 {
			return stack
		}
	}
	return nil
}

// zerologErrorMarshaller implements zerolog's LogObjectMarshaler interface
// to provide structured serialization of aerr errors.
type zerologErrorMarshaller struct {
	err error
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler for high-performance zerolog integration.
func (m zerologErrorMarshaller) MarshalZerologObject(evt *zerolog.Event) {
	if m.err == nil {
		return
	}

	aErr, ok := aerr.AsAerr(m.err)
	if !ok {
		return
	}

	if code := aErr.GetCode(); code != "" {
		evt.Str("code", code)
	}

	if attributes := aErr.GetAttributes(); len(attributes) > 0 {
		dict := zerolog.Dict()
		for k, v := range attributes {
			switch vTyped := v.(type) {
			case nil:
				// Skip nil values
			case error:
				dict = dict.Str(k, vTyped.Error())
			default:
				dict = dict.Interface(k, vTyped)
			}
		}
		evt.Dict("attributes", dict)
	}
}
