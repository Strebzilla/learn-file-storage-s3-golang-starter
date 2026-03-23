package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	cmdResult := bytes.Buffer{}
	cmd.Stdout = &cmdResult

	err := cmd.Run()
	if err != nil {
		return "", err
	}
	
	return outputFilePath, nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	type parameters struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name,omitempty"`
		CodecLongName      string `json:"codec_long_name,omitempty"`
		Profile            string `json:"profile,omitempty"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
		CodedWidth         int    `json:"coded_width,omitempty"`
		CodedHeight        int    `json:"coded_height,omitempty"`
		ClosedCaptions     int    `json:"closed_captions,omitempty"`
		FilmGrain          int    `json:"film_grain,omitempty"`
		HasBFrames         int    `json:"has_b_frames,omitempty"`
		SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
		DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
		PixFmt             string `json:"pix_fmt,omitempty"`
		Level              int    `json:"level,omitempty"`
		ColorRange         string `json:"color_range,omitempty"`
		ColorSpace         string `json:"color_space,omitempty"`
		ColorTransfer      string `json:"color_transfer,omitempty"`
		ColorPrimaries     string `json:"color_primaries,omitempty"`
		ChromaLocation     string `json:"chroma_location,omitempty"`
		FieldOrder         string `json:"field_order,omitempty"`
		Refs               int    `json:"refs,omitempty"`
		IsAvc              string `json:"is_avc,omitempty"`
		NalLengthSize      string `json:"nal_length_size,omitempty"`
		ID                 string `json:"id"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate,omitempty"`
		BitsPerRawSample   string `json:"bits_per_raw_sample,omitempty"`
		NbFrames           string `json:"nb_frames"`
		ExtradataSize      int    `json:"extradata_size"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
			NonDiegetic     int `json:"non_diegetic"`
			Captions        int `json:"captions"`
			Descriptions    int `json:"descriptions"`
			Metadata        int `json:"metadata"`
			Dependent       int `json:"dependent"`
			StillImage      int `json:"still_image"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags"`
		SampleFmt      string `json:"sample_fmt,omitempty"`
		SampleRate     string `json:"sample_rate,omitempty"`
		Channels       int    `json:"channels,omitempty"`
		ChannelLayout  string `json:"channel_layout,omitempty"`
		BitsPerSample  int    `json:"bits_per_sample,omitempty"`
		InitialPadding int    `json:"initial_padding,omitempty"`
	} `json:"streams"`
}
	
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	cmdResult := bytes.Buffer{}
	cmd.Stdout = &cmdResult

	err := cmd.Run()
	if err != nil {
		return "", err
	}
	
	cmdResult.Bytes()

	
	params := parameters{}
	err = json.Unmarshal(cmdResult.Bytes(), &params)

	if err != nil {
		return "", err
	}
	
	aspectRatio := float64(params.Streams[0].Width) / float64(params.Streams[0].Height)
	fmt.Println("Width:", params.Streams[0].Width, "Height", params.Streams[0].Height)
	fmt.Println("Aspect Ratio", aspectRatio)
	if aspectRatio > 1.7 && aspectRatio < 1.8 {
		return "16:9", nil
	}
	if aspectRatio > 0.4 && aspectRatio < 0.6 {
		return "9:16", nil
	}
	return "other", nil
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 1 << 30 // 1 GB

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		fmt.Println("Could not find video in database: ", err)
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}

	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video not owned by user", err)
		return
	}

	videoFile, videoHeader, err := r.FormFile("video")
	if err != nil {
		fmt.Println("Could not get file and header: ", err)
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}
	defer videoFile.Close()

	contentType := videoHeader.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(contentType)
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		fmt.Println("Cloud not create file", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	io.Copy(tempFile, videoFile)
	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		fmt.Println("Cloud not process for fast start", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}
	err = os.Remove(tempFile.Name())
	if err != nil {
		fmt.Println("Cloud not delete preprocessed file", err)
	}
	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		fmt.Println("Error opening processed file", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(processedFilePath)
	if err != nil {
		fmt.Println("Could not get aspect ratio:", err)
	}

	// Set the tempfile point to the beginning of the file so we can read it again
	tempFile.Seek(0, io.SeekStart)

	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	if err != nil {
		fmt.Println("Cloud not read rand", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}
	fileName := base64.RawURLEncoding.EncodeToString(nonce) + ".mp4"

	s3Prefix := "other/"
	if aspectRatio == "16:9" {
		s3Prefix = "landscape/"
	}
	if aspectRatio == "9:16" {
		s3Prefix = "portrait/"
	}

	s3Key := s3Prefix + fileName
	
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &s3Key,
		ContentType: &contentType,
		Body:processedFile,
	})
	if err != nil {
		fmt.Println("Could not PutObject", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}
	
	s3URL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, s3Key)
	videoMetadata.VideoURL = &s3URL
	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		fmt.Println("Could not update video metadata: ", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}
	

	respondWithJSON(w, http.StatusOK, videoMetadata)
}
