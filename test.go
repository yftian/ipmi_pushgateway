package main

import (
	log "github.com/cihub/seelog"
)

func main() {
	defer log.Flush()

	logger, err := log.LoggerFromConfigAsFile("./conf/logconf.xml")
	if err != nil {
		log.Errorf("parse config.xml error: %v", err)
		return
	}

	log.ReplaceLogger(logger)

	log.Info("seelog test begin")

	for i := 0; i < 1; i++ {
		log.Info("3333333")
		log.Tracef("hello seelog trace, i = %d", i)
		log.Debugf("hello seelog debug, i = %d", i)
		log.Infof("hello seelog info, i = %d", i)
		log.Warnf("hello seelog warn, i = %d", i)
		log.Errorf("hello seelog error, i = %d", i)
		log.Criticalf("hello seelog critical, i = %d", i)
	}

	log.Debug("seelog test end")
}