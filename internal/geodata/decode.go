package geodata

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// decodeGeoSiteList mem-parse protobuf GeoSiteList secara manual.
func decodeGeoSiteList(data []byte) (map[string][]Domain, error) {
	out := make(map[string][]Domain)

	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, errors.New("geodata: malformed tag")
		}
		data = data[n:]

		if num != 1 || typ != protowire.BytesType {
			m := protowire.ConsumeFieldValue(num, typ, data)
			if m < 0 {
				return nil, errors.New("geodata: malformed field")
			}
			data = data[m:]
			continue
		}

		entry, n := protowire.ConsumeBytes(data)
		if n < 0 {
			return nil, errors.New("geodata: malformed entry")
		}
		data = data[n:]

		cc, domains, err := decodeGeoSite(entry)
		if err != nil {
			return nil, err
		}
		if cc != "" {
			out[cc] = domains
		}
	}
	return out, nil
}

func decodeGeoSite(b []byte) (string, []Domain, error) {
	var cc string
	var domains []Domain

	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", nil, errors.New("geodata: malformed geosite tag")
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.BytesType:
			v, m := protowire.ConsumeString(b)
			if m < 0 {
				return "", nil, errors.New("geodata: bad country_code")
			}
			cc = v
			b = b[m:]
		case num == 2 && typ == protowire.BytesType:
			d, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return "", nil, errors.New("geodata: bad domain")
			}
			dom, err := decodeDomain(d)
			if err != nil {
				return "", nil, err
			}
			domains = append(domains, dom)
			b = b[m:]
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return "", nil, fmt.Errorf("geodata: unknown field %d", num)
			}
			b = b[m:]
		}
	}
	return cc, domains, nil
}

func decodeDomain(b []byte) (Domain, error) {
	var d Domain
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return d, errors.New("geodata: malformed domain tag")
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return d, errors.New("geodata: bad domain type")
			}
			d.Type = DomainType(v)
			b = b[m:]
		case num == 2 && typ == protowire.BytesType:
			v, m := protowire.ConsumeString(b)
			if m < 0 {
				return d, errors.New("geodata: bad domain value")
			}
			d.Value = v
			b = b[m:]
		case num == 3 && typ == protowire.BytesType:
			attr, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return d, errors.New("geodata: bad attr")
			}
			if key := decodeAttrKey(attr); key != "" {
				d.Attrs = append(d.Attrs, key)
			}
			b = b[m:]
		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return d, errors.New("geodata: unknown domain field")
			}
			b = b[m:]
		}
	}
	return d, nil
}

func decodeAttrKey(b []byte) string {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return ""
		}
		b = b[n:]
		if num == 1 && typ == protowire.BytesType {
			v, m := protowire.ConsumeString(b)
			if m < 0 {
				return ""
			}
			return v
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return ""
		}
		b = b[m:]
	}
	return ""
}