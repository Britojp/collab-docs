package br.ufg.collabdocs.operation;

import br.ufg.collabdocs.config.RabbitMQConfig;
import br.ufg.collabdocs.document.DocumentService;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.util.UUID;

@Component
public class OperationConsumer {

    private static final Logger log = LoggerFactory.getLogger(OperationConsumer.class);

    private final OperationRepository repository;
    private final DocumentService documents;

    public OperationConsumer(OperationRepository repository, DocumentService documents) {
        this.repository = repository;
        this.documents = documents;
    }

    @RabbitListener(queues = RabbitMQConfig.Q_PERSIST)
    @Transactional
    public void onOperation(OpEvent event) {
        try {
            repository.save(Operation.from(event));
            documents.applyOperation(event);
            log.debug("persisted op doc={} v={} type={}", event.docId(), event.version(), event.type());
        } catch (Exception e) {
            log.error("failed to persist op: {}", e.getMessage());
            throw e; // re-queue on failure
        }
    }
}
