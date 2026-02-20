// C ABI wrapper for QubicDB's vector loader.
// Exposes the symbols expected by pkg/vector/loader.go.
// Adapted from kelindar/search (MIT License): https://github.com/kelindar/search

#include "common.h"
#include "llama.h"
#include "ggml.h"
#include <vector>
#include <cstdio>

typedef struct llama_model*   model_t;
typedef struct llama_context* context_t;

static int32_t embd_normalize = 2; // 2 = euclidean

static void batch_add_seq(llama_batch & batch, const std::vector<int32_t> & tokens, llama_seq_id seq_id) {
    for (size_t i = 0; i < tokens.size(); i++) {
        common_batch_add(batch, tokens[i], i, { seq_id }, true);
    }
}

static int batch_decode(llama_context * ctx, llama_batch & batch, float * output, int n_seq, int n_embd, int embd_norm) {
    const enum llama_pooling_type pooling_type = llama_pooling_type(ctx);
    const struct llama_model * model = llama_get_model(ctx);

    llama_memory_clear(llama_get_memory(ctx), true);

    if (llama_model_has_encoder(model) && !llama_model_has_decoder(model)) {
        if (llama_encode(ctx, batch) < 0) {
            return -1;
        }
    } else if (!llama_model_has_encoder(model) && llama_model_has_decoder(model)) {
        if (llama_decode(ctx, batch) < 0) {
            return -1;
        }
    }

    for (int i = 0; i < batch.n_tokens; i++) {
        if (!batch.logits[i]) {
            continue;
        }

        const float * embd = nullptr;
        int embd_pos = 0;

        if (pooling_type == LLAMA_POOLING_TYPE_NONE) {
            embd     = llama_get_embeddings_ith(ctx, i);
            embd_pos = i;
            GGML_ASSERT(embd != NULL && "Failed to get token embeddings");
        } else {
            embd     = llama_get_embeddings_seq(ctx, batch.seq_id[i][0]);
            embd_pos = batch.seq_id[i][0];
            GGML_ASSERT(embd != NULL && "Failed to get sequence embeddings");
        }

        float * out = output + embd_pos * n_embd;
        common_embd_normalize(embd, out, n_embd, embd_norm);
    }
    return 0;
}

extern "C" {

    // Initialise the llama.cpp backend.
    // log_level: 0=DEBUG 1=INFO 2=WARN 3=ERROR 4=NONE
    LLAMA_API void load_library(ggml_log_level desired) {
        llama_backend_init();
        llama_numa_init(GGML_NUMA_STRATEGY_DISTRIBUTE);

        auto desired_ptr = new ggml_log_level;
        *desired_ptr = desired;
        llama_log_set([](ggml_log_level level, const char* text, void* user_data) {
            if (level < *(ggml_log_level*)user_data) {
                return;
            }
            fputs(text, stderr);
            fflush(stderr);
        }, desired_ptr);
    }

    // Load a GGUF model file.
    LLAMA_API model_t load_model(const char * path_model, const uint32_t n_gpu_layers) {
        struct llama_model_params params = llama_model_default_params();
        params.n_gpu_layers = n_gpu_layers;
        return llama_model_load_from_file(path_model, params);
    }

    // Free a loaded model.
    LLAMA_API void free_model(model_t model) {
        llama_model_free(model);
    }

    // Create an embedding context for the model.
    LLAMA_API context_t load_context(model_t model, const uint32_t ctx_size, const bool embeddings) {
        struct llama_context_params params = llama_context_default_params();
        params.n_ctx    = ctx_size;
        params.n_batch  = ctx_size;
        params.n_ubatch = ctx_size;
        params.embeddings  = embeddings;
        return llama_init_from_model(model, params);
    }

    // Free a context.
    LLAMA_API void free_context(context_t ctx) {
        llama_free(ctx);
    }

    // Return the embedding dimension of the model, or -1 if not supported.
    LLAMA_API int32_t embed_size(model_t model) {
        if (llama_model_has_encoder(model) && llama_model_has_decoder(model)) {
            return -1;
        }
        return llama_model_n_embd(model);
    }

    // Embed text and write the result into out_embeddings.
    // Returns 0 on success, non-zero on error.
    LLAMA_API int embed_text(context_t ctx, const char* text, float* out_embeddings, uint32_t* out_tokens) {
        const enum llama_pooling_type pooling_type = llama_pooling_type(ctx);
        model_t model = (model_t)llama_get_model(ctx);
        const uint64_t n_batch = llama_n_batch(ctx);

        auto inp = common_tokenize(ctx, text, true, true);
        *out_tokens = inp.size();
        if (inp.size() > n_batch) {
            printf("Number of tokens exceeds batch size, increase batch size\n");
            return 1;
        }

        if (inp.empty() || inp.back() != llama_vocab_sep(llama_model_get_vocab(model))) {
            return 2;
        }

        struct llama_batch batch = llama_batch_init(n_batch, 0, 1);
        batch_add_seq(batch, inp, 0);

        const int n_embd = llama_model_n_embd(model);
        if (batch_decode(ctx, batch, out_embeddings, 1, n_embd, embd_normalize) != 0) {
            llama_batch_free(batch);
            return 3;
        }

        llama_batch_free(batch);
        return 0;
    }

} // extern "C"
