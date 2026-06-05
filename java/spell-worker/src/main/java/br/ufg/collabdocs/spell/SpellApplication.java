package br.ufg.collabdocs.spell;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class SpellApplication implements CommandLineRunner {

    public static void main(String[] args) {
        SpringApplication.run(SpellApplication.class, args);
    }

    @Override
    public void run(String... args) throws InterruptedException {
        Thread.currentThread().join();
    }
}
