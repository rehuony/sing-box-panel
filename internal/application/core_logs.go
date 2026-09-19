// SPDX-License-Identifier: GPL-3.0-or-later
package application

import "github.com/rehuony/sing-box-panel/internal/corelogs"

func (application *Application) CoreLogFiles() ([]corelogs.File, error) {
	files, err := corelogs.New(application.settings.DataDir)
	if err != nil {
		return nil, err
	}
	return files.List()
}
func (application *Application) CoreLogContent(name string, offset int64) (corelogs.Chunk, error) {
	files, err := corelogs.New(application.settings.DataDir)
	if err != nil {
		return corelogs.Chunk{}, err
	}
	return files.Read(name, offset)
}
