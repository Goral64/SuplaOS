package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// logKind is a kind of measurement logs of a channel: the table and how
// the fields of the API map to its columns.
type logKind struct {
	table    string
	logsType string
	// fields of the API in the order of cols.
	fields, cols []string
	// milli: the API gives the column divided by 1000.
	milli string
}

func same(cols ...string) ([]string, []string) { return cols, cols }

var (
	emCols = []string{"phase1_fae", "phase1_rae", "phase1_fre", "phase1_rre", "phase2_fae", "phase2_rae",
		"phase2_fre", "phase2_rre", "phase3_fae", "phase3_rae", "phase3_fre", "phase3_rre", "fae_balanced", "rae_balanced"}
	phaseFields = []string{"phaseNo", "min", "max", "avg"}
	phaseCols   = []string{"phase_no", "min", "max", "avg"}
)

// logKinds are the kinds of logs of a channel function (supla-cloud,
// ChannelMeasurementLogsController).
func logKinds(function int) []logKind {
	k := func(table string, fields, cols []string) logKind {
		return logKind{table: table, fields: fields, cols: cols}
	}
	switch function {
	case 40: // THERMOMETER
		f, c := same("temperature")
		return []logKind{k("supla_temperature_log", f, c)}
	case 42: // HUMIDITY
		f, c := same("humidity")
		return []logKind{k("supla_temphumidity_log", f, c)}
	case 45: // HUMIDITYANDTEMPERATURE
		f, c := same("temperature", "humidity")
		return []logKind{k("supla_temphumidity_log", f, c)}
	case 310: // ELECTRICITYMETER
		ab := logKind{table: "supla_em_voltage_aberration_log", logsType: "voltageAberrations",
			fields: []string{"phaseNo", "countTotal", "countAbove", "countBelow", "secAbove", "secBelow",
				"maxSecAbove", "maxSecBelow", "minVoltage", "maxVoltage", "avgVoltage", "measurementTimeSec"},
			cols: []string{"phase_no", "count_total", "count_above", "count_below", "sec_above", "sec_below",
				"max_sec_above", "max_sec_below", "min_voltage", "max_voltage", "avg_voltage", "measurement_time_sec"}}
		return []logKind{
			k("supla_em_log", emCols, emCols), ab,
			{table: "supla_em_voltage_log", logsType: "voltageHistory", fields: phaseFields, cols: phaseCols},
			{table: "supla_em_current_log", logsType: "currentHistory", fields: phaseFields, cols: phaseCols},
			{table: "supla_em_power_active_log", logsType: "powerActiveHistory", fields: phaseFields, cols: phaseCols},
		}
	case 315, 320, 330, 340: // IC_ELECTRICITYMETER, IC_GASMETER, IC_WATERMETER, IC_HEATMETER
		f, c := same("counter", "calculated_value")
		l := k("supla_ic_log", f, c)
		l.milli = "calculated_value"
		return []logKind{l}
	case 400, 410: // THERMOSTAT, THERMOSTATHEATPOLHOMEPLUS
		f, c := same("on", "measured_temperature", "preset_temperature")
		return []logKind{k("supla_thermostat_log", f, c)}
	case 520: // GENERAL_PURPOSE_MEASUREMENT
		f, c := same("open_value", "close_value", "avg_value", "max_value", "min_value")
		return []logKind{k("supla_gp_measurement_log", f, c)}
	case 530: // GENERAL_PURPOSE_METER
		f, c := same("value")
		return []logKind{k("supla_gp_meter_log", f, c)}
	}
	return nil
}

// logsPage is the number of log items read at once (the limit of the
// Cloud) and written in one INSERT.
const (
	logsPage   = 10000
	insertRows = 1000
)

// logs reads a page of measurement logs of a channel, the oldest first.
func (s *Source) logs(channel int, logsType string, after int64, offset int) ([]map[string]json.RawMessage, error) {
	q := url.Values{"order": {"ASC"}, "limit": {strconv.Itoa(logsPage)}, "offset": {strconv.Itoa(offset)}}
	if after > 0 {
		q.Set("afterTimestamp", strconv.FormatInt(after, 10))
	}
	if logsType != "" {
		q.Set("logsType", logsType)
	}
	var items []map[string]json.RawMessage
	err := s.get(fmt.Sprintf("channels/%d/measurement-logs?%s", channel, q.Encode()), &items)
	return items, err
}

// sqlValue is a JSON value of a log item as SQL.
func sqlValue(v json.RawMessage, milli bool) (string, error) {
	s := string(bytes.TrimSpace(v))
	switch {
	case s == "" || s == "null":
		return "NULL", nil
	case s == "true":
		return "1", nil
	case s == "false":
		return "0", nil
	case milli:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(math.Round(f*1000), 'f', -1, 64), nil
	}
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return "", fmt.Errorf("not a number: %s", s)
	}
	return s, nil
}

// insertSQL writes log items of a channel as INSERT IGNORE statements:
// the rows that are there already (the same time) are skipped.
func insertSQL(k logKind, channel int, items []map[string]json.RawMessage) (string, error) {
	var sb strings.Builder
	cols := "channel_id, `date`, `" + strings.Join(k.cols, "`, `") + "`"
	for i := 0; i < len(items); i += insertRows {
		end := min(i+insertRows, len(items))
		fmt.Fprintf(&sb, "INSERT IGNORE INTO %s (%s) VALUES\n", k.table, cols)
		for j, item := range items[i:end] {
			ts, err := strconv.ParseInt(strings.Trim(string(item["date_timestamp"]), `"`), 10, 64)
			if err != nil {
				return "", fmt.Errorf("log item without time: %v", err)
			}
			vals := []string{strconv.Itoa(channel), fmt.Sprintf("FROM_UNIXTIME(%d)", ts)}
			for _, f := range k.fields {
				v, err := sqlValue(item[f], f == k.milli)
				if err != nil {
					return "", fmt.Errorf("%s: %v", f, err)
				}
				vals = append(vals, v)
			}
			sep := ",\n"
			if j == end-i-1 {
				sep = ";\n"
			}
			sb.WriteString("(" + strings.Join(vals, ", ") + ")" + sep)
		}
	}
	return sb.String(), nil
}

// ImportHistory copies the measurement logs of the channels of the bundle
// from the source, from the newest one already here on, and returns how
// many items were read. It can be run again later to add what the source
// logged until the devices were switched.
func ImportHistory(src *Source, db DB, b *Bundle, log io.Writer) (int, error) {
	total := 0
	for _, d := range b.Devices {
		for _, c := range d.Channels {
			for _, k := range logKinds(c.FunctionID) {
				rows, err := db.Query(fmt.Sprintf("SET time_zone = '+00:00'; SELECT IFNULL(UNIX_TIMESTAMP(MAX(`date`)), 0) FROM %s WHERE channel_id = %d;", k.table, c.ID))
				if err != nil {
					return total, err
				}
				var after int64
				if len(rows) > 0 {
					after, _ = strconv.ParseInt(rows[0][0], 10, 64)
				}
				n := 0
				for offset := 0; ; offset += logsPage {
					items, err := src.logs(c.ID, k.logsType, after, offset)
					if err != nil {
						return total, fmt.Errorf("channel %d: %w", c.ID, err)
					}
					if len(items) == 0 {
						break
					}
					q, err := insertSQL(k, c.ID, items)
					if err != nil {
						return total, fmt.Errorf("channel %d: %w", c.ID, err)
					}
					if _, err := db.Query("SET time_zone = '+00:00';\n" + q); err != nil {
						return total, fmt.Errorf("channel %d: %w", c.ID, err)
					}
					n += len(items)
					if len(items) < logsPage {
						break
					}
				}
				if n > 0 {
					fmt.Fprintf(log, "Channel %d: %d log items (%s)\n", c.ID, n, strings.TrimPrefix(k.table, "supla_"))
				}
				total += n
			}
		}
	}
	return total, nil
}
