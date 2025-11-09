package sonic

import "encoding/json"

func Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func MarshalString(v interface{}) (string, error) {
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ToString(v interface{}) (string, error) {
	return MarshalString(v)
}
