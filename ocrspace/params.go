package ocrspace

import "errors"

func WithApiKey(apiKey string) configFunc {
	return func(p *Config) error {
		if apiKey == "" {
			p.ApiKey = "helloworld"
		} else {
			p.ApiKey = apiKey
		}
		return nil
	}
}

/*
Arabic=ara
Bulgarian=bul
Chinese(Simplified)=chs
Chinese(Traditional)=cht
Croatian = hrv
Czech = cze
Danish = dan
Dutch = dut
English = eng
Finnish = fin
French = fre
German = ger
Greek = gre
Hungarian = hun
Korean = kor
Italian = ita
Japanese = jpn
Polish = pol
Portuguese = por
Russian = rus
Slovenian = slv
Spanish = spa
Swedish = swe
Thai = tha
Turkish = tur
Ukrainian = ukr
Vietnamese = vnm
AUTODETECT LANGUAGE = auto
*/
func WithLanguage(lang string) configFunc {
	return func(p *Config) error {
		p.Language = &lang
		return nil
	}
}

func WithIsOverlayRequired(isOverlayRequired bool) configFunc {
	return func(p *Config) error {
		p.IsOverlayRequired = &isOverlayRequired
		return nil
	}
}

func WithFiletype(filetype string) configFunc {
	return func(p *Config) error {
		p.Filetype = &filetype
		return nil
	}
}

func WithDetectOrientation(detectOrientation bool) configFunc {
	return func(p *Config) error {
		p.DetectOrientation = &detectOrientation
		return nil
	}
}

func WithIsCreateSearchablePdf(isCreateSearchablePdf bool) configFunc {
	return func(p *Config) error {
		p.IsCreateSearchablePdf = &isCreateSearchablePdf
		return nil
	}
}

func WithIsSearchablePdfHideTextLayer(isSearchablePdfHideTextLayer bool) configFunc {
	return func(p *Config) error {
		p.IsSearchablePdfHideTextLayer = &isSearchablePdfHideTextLayer
		return nil
	}
}

func WithScale(scale bool) configFunc {
	return func(p *Config) error {
		p.Scale = &scale
		return nil
	}
}

func WithIsTable(isTable bool) configFunc {
	return func(p *Config) error {
		p.IsTable = &isTable
		return nil
	}
}

var ErrInvalidParameter = errors.New("invalid parameter")

func WithOCREngine(ocrEngine int) configFunc {
	return func(p *Config) error {
		if ocrEngine != 1 && ocrEngine != 2 {
			return ErrInvalidParameter
		}
		p.OCREngine = &ocrEngine
		return nil
	}
}
