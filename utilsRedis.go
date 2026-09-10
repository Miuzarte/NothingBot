package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/redis/rueidis"
)

type RedisClient struct {
	Client rueidis.Client
}

func (r *RedisClient) Set(key string, value any, expiration time.Duration) error {
	var v string
	switch value := value.(type) {
	case string:
		v = value
	case []byte:
		v = string(value)
	default:
		vv, err := json.Marshal(value)
		if err != nil {
			return err
		}
		v = string(vv)
	}
	return r.Client.Do(context.Background(), r.Client.B().Set().Key(key).Value(v).Ex(expiration).Build()).Error()
}

func (r *RedisClient) MSet(kvs map[string]any) error {
	for key, value := range kvs {
		switch v := value.(type) {
		case string:
		case []byte:
			kvs[key] = string(v)
		default:
			vv, err := json.Marshal(value)
			if err != nil {
				return err
			}
			kvs[key] = string(vv)
		}
	}
	cmd := r.Client.B().Mset().KeyValue()
	for k, v := range kvs {
		cmd.KeyValue(k, v.(string))
	}
	return r.Client.Do(context.Background(), cmd.Build()).Error()
}

func (r *RedisClient) Get(key string) RedisResult {
	data, err := r.Client.Do(context.Background(), r.Client.B().Get().Key(key).Build()).ToString()
	return RedisResult{Data: data, Err: err}
}

func (r *RedisClient) MGet(keys ...string) RedisResultList {
	data, err := r.Client.Do(context.Background(), r.Client.B().Mget().Key(keys...).Build()).AsStrSlice()
	return RedisResultList{Data: data, Err: err}
}

func (r *RedisClient) ScanGet(match string) RedisResult {
	var cursor uint64
	for {
		se, err := r.Client.Do(context.Background(), r.Client.B().Scan().Cursor(cursor).Match(match).Build()).AsScanEntry()
		if err != nil {
			return RedisResult{Err: err}
		}
		cursor = se.Cursor
		for _, key := range se.Elements {
			return r.Get(key)
		}
		if cursor == 0 {
			break
		}
	}
	return RedisResult{}
}

func (r *RedisClient) LPush(key string, value any) error {
	var values []string
	switch value := value.(type) {
	case string:
		values = []string{value}
	case []string:
		values = value
	case []byte:
		values = []string{string(value)}
	case [][]byte:
		values := make([]string, len(value))
		for i := range value {
			values[i] = string(value[i])
		}
	default:
		vv, err := json.Marshal(value)
		if err != nil {
			return err
		}
		values = []string{string(vv)}
	}
	return r.Client.Do(context.Background(), r.Client.B().Lpush().Key(key).Element(values...).Build()).Error()
}

func (r *RedisClient) LTrim(key string, start, stop int64) error {
	return r.Client.Do(context.Background(), r.Client.B().Ltrim().Key(key).Start(start).Stop(stop).Build()).Error()
}

func (r *RedisClient) LLen(key string) (int64, error) {
	return r.Client.Do(context.Background(), r.Client.B().Llen().Key(key).Build()).AsInt64()
}

func (r *RedisClient) LRange(key string, start, stop int64) RedisResultList {
	data, err := r.Client.Do(context.Background(), r.Client.B().Lrange().Key(key).Start(start).Stop(stop).Build()).AsStrSlice()
	return RedisResultList{Data: data, Err: err}
}

type RedisResult struct {
	Data string
	Err  error
}

func (rr *RedisResult) IsNil() bool {
	return rr == nil || rueidis.IsRedisNil(rr.Err) || rr.Data == ""
}

func UnmarshalResult[T any](rr RedisResult) (to *T, err error) {
	if rr.Err != nil {
		return nil, rr.Err
	}
	to = new(T)
	return to, json.Unmarshal([]byte(rr.Data), to)
}

type RedisResultList struct {
	Data []string
	Err  error
}

func (rr *RedisResultList) IsNil() bool {
	return rr == nil || rueidis.IsRedisNil(rr.Err) || len(rr.Data) == 0
}

func UnmarshalResultList[T any](results RedisResultList, to *T) (alsoTo *T, err error) {
	if results.Err != nil {
		return nil, results.Err
	}
	toType := reflect.TypeOf(*to)
	if toType.Kind() == reflect.Slice || toType.Kind() == reflect.Array {
		sliceValue := reflect.MakeSlice(toType, len(results.Data), len(results.Data))
		for i, data := range results.Data {
			elem := sliceValue.Index(i).Addr().Interface()
			if err := json.Unmarshal([]byte(data), elem); err != nil {
				return nil, err
			}
		}
		reflect.ValueOf(to).Elem().Set(sliceValue)
		return to, nil
	}
	return nil, errors.New("to is not a list type")
}
