package br.ufg.collabdocs.analytics;

import br.ufg.collabdocs.analytics.proto.AnalyticsServiceGrpc;
import br.ufg.collabdocs.analytics.proto.DocumentAnalyticsRequest;
import br.ufg.collabdocs.analytics.proto.DocumentAnalyticsResponse;
import io.grpc.Status;
import io.grpc.stub.StreamObserver;
import net.devh.boot.grpc.server.service.GrpcService;

import java.util.UUID;

@GrpcService
public class AnalyticsGrpcService extends AnalyticsServiceGrpc.AnalyticsServiceImplBase {

    private final AnalyticsService analytics;

    public AnalyticsGrpcService(AnalyticsService analytics) {
        this.analytics = analytics;
    }

    @Override
    public void getDocumentAnalytics(
            DocumentAnalyticsRequest request,
            StreamObserver<DocumentAnalyticsResponse> responseObserver) {
        try {
            DocumentAnalytics result = analytics.getDocumentAnalytics(UUID.fromString(request.getDocId()));
            responseObserver.onNext(DocumentAnalyticsResponse.newBuilder()
                    .setDocId(result.docId().toString())
                    .setCharCount(result.charCount())
                    .setWordCount(result.wordCount())
                    .setLineCount(result.lineCount())
                    .setParagraphCount(result.paragraphCount())
                    .setVersion(result.version())
                    .setLastActivity(result.lastActivity().toString())
                    .build());
            responseObserver.onCompleted();
        } catch (IllegalArgumentException e) {
            responseObserver.onError(Status.INVALID_ARGUMENT
                    .withDescription("invalid doc_id")
                    .withCause(e)
                    .asRuntimeException());
        } catch (Exception e) {
            responseObserver.onError(Status.NOT_FOUND
                    .withDescription(e.getMessage())
                    .withCause(e)
                    .asRuntimeException());
        }
    }
}
