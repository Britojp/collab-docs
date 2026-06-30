package br.ufg.collabdocs.worker;

import br.ufg.collabdocs.config.RabbitMQConfig;
import br.ufg.collabdocs.metrics.Metric;
import br.ufg.collabdocs.metrics.MetricRepository;
import br.ufg.collabdocs.operation.OpEvent;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.UUID;

@Component
public class MetricWorker {

    private static final Logger log = LoggerFactory.getLogger(MetricWorker.class);

    private final MetricRepository metrics;

    public MetricWorker(MetricRepository metrics) {
        this.metrics = metrics;
    }

    @RabbitListener(queues = RabbitMQConfig.Q_METRIC)
    @Transactional
    public void onOperation(OpEvent event) {
        UUID docId = UUID.fromString(event.docId());

        Metric m = metrics.findById(docId).orElseGet(() -> {
            var fresh = new Metric();
            fresh.setDocId(docId);
            return fresh;
        });

        m.setTotalOps(m.getTotalOps() + 1);
        if ("insert".equals(event.normalizedType())) {
            m.setCharsInserted(m.getCharsInserted() + 1);
        } else if ("delete".equals(event.normalizedType())) {
            m.setCharsDeleted(m.getCharsDeleted() + 1);
        }
        m.setLastActivity(LocalDateTime.now());
        metrics.save(m);

        log.debug("metric updated doc={} totalOps={}", docId, m.getTotalOps());
    }
}
