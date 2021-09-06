package CodeGenerator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

type Project struct {
	UserId  string  `header:"id"`
	Config  Config  `json:"config"`
	DataSet DataSet `json:"dataset"`
	Content Content `json:"content"`
}

type Train struct {
	Config  Config  `json:"config"`
	DataSet DataSet `json:"dataset"`
	UserId  string  `json:"id"`
}

const (
	importTf    = "import tensorflow as tf\n\n"
	importTfa   = "import tensorflow_addons as tfa\n\n"
	tf          = "tf"
	tfa         = "tfa"
	keras       = ".keras"
	layers      = ".layers"
	math        = ".math"
	createModel = "model = tf.keras.Model(inputs=%s, outputs=%s)\n\n"
)

func (p *Project) BindProject(r *http.Request) error {
	data := make(map[string]json.RawMessage)
	cc := make(map[string]json.RawMessage)

	// Binding request body
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		return err
	}

	p.UserId = r.Header.Get("id")

	// Unmarshalling Config.
	var config map[string]json.RawMessage
	err = json.Unmarshal(data["config"], &config)
	if err != nil {
		return err
	}

	err = p.Config.UnmarshalConfig(config)
	if err != nil {
		return err
	}

	p.DataSet.Bind(data["dataset"])

	// Unmarshalling Content.
	err = json.Unmarshal(data["content"], &cc)
	if err != nil {
		return err
	}

	err = p.Content.BindContent(cc)
	if err != nil {
		return err
	}

	return nil
}

func (p *Project) SaveModel() error {
	err := p.GenerateModel()
	if err != nil {
		return err
	}

	err = p.SaveModel()
	if err != nil {
		return err
	}

	cmd := exec.Command("python", "train.py")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return nil
	}
	fmt.Println("Finished saving model %s", p.UserId)

	return nil
}

func (p *Project) GenerateModel() error {
	var codes []string
	codes = append(codes, importTf)
	codes = append(codes, importTfa)

	Layers, err := p.Content.GenLayers()
	if err != nil {
		return err
	}
	codes = append(codes, Layers...)

	Configs, err := p.Config.GenConfig()
	if err != nil {
		return err
	}
	codes = append(codes, Configs...)

	// create python file
	err = MakeTextFile(codes, "model.py")

	return nil
}

func (p *Project) GenerateSaveModel() error {
	var codes []string
	codes = append(codes, importTf)
	codes = append(codes, importTfa)
	codes = append(codes, "import model\n\n")

	// Python comment.
	saveCode := fmt.Sprintf("model.model.save('./%s/Model')", p.UserId)
	codes = append(codes, saveCode)

	// Generate train python file
	err := MakeTextFile(codes, "train.py")
	if err != nil {
		return err
	}

	return nil
}

func (p *Project) GetTrainBody() Train {
	return Train{DataSet: p.DataSet, UserId: p.UserId, Config: p.Config}
}