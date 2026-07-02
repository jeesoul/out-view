package com.outview.webrtc;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;

@JsonInclude(JsonInclude.Include.NON_NULL)
public class IPCMessage {
    @JsonProperty("type")
    private String type;

    @JsonProperty("payload")
    private JsonNode payload;

    public IPCMessage() {}

    public IPCMessage(String type, JsonNode payload) {
        this.type = type;
        this.payload = payload;
    }

    public String getType() { return type; }
    public void setType(String type) { this.type = type; }

    public JsonNode getPayload() { return payload; }
    public void setPayload(JsonNode payload) { this.payload = payload; }
}
