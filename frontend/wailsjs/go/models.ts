export namespace main {
	
	export class ExportOptions {
	    startTime: number;
	    endTime: number;
	    removeAudio: boolean;
	    filename: string;
	    outputDir: string;
	    qualityPreset: string;
	    maxResolution: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startTime = source["startTime"];
	        this.endTime = source["endTime"];
	        this.removeAudio = source["removeAudio"];
	        this.filename = source["filename"];
	        this.outputDir = source["outputDir"];
	        this.qualityPreset = source["qualityPreset"];
	        this.maxResolution = source["maxResolution"];
	    }
	}
	export class VideoInfo {
	    id: string;
	    title: string;
	    author: string;
	    duration: number;
	    thumbnail: string;
	    videoUrl: string;
	    sourceWidth: number;
	    sourceHeight: number;
	    previewWidth: number;
	    previewHeight: number;
	    downloadMethod: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.author = source["author"];
	        this.duration = source["duration"];
	        this.thumbnail = source["thumbnail"];
	        this.videoUrl = source["videoUrl"];
	        this.sourceWidth = source["sourceWidth"];
	        this.sourceHeight = source["sourceHeight"];
	        this.previewWidth = source["previewWidth"];
	        this.previewHeight = source["previewHeight"];
	        this.downloadMethod = source["downloadMethod"];
	    }
	}

}

