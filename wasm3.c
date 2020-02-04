#include "_cgo_export.h"

// module_get_function is a helper function for the module Go struct
IM3Function module_get_function(IM3Module i_module, int index) {
	IM3Function f = & i_module->functions [index];
	return f;
}

// module_get_imported_function is a helper function for the module Go struct
IM3Function module_get_imported_function(IM3Module i_module, int index) {
    if(i_module==NULL)
        return NULL;
    
    if(i_module->imports == NULL)
        return NULL;

	return i_module->imports[index];
}

int findFunction(IM3Function * o_function, IM3Runtime i_runtime, const char * const i_moduleName, const char * const i_functionName)
{
    void * r = NULL;
    *o_function = NULL;

    IM3Module i_module = i_runtime->modules;

    while (i_module)
    {
        if (i_module->name)
        {
            if (strcmp (i_module->name, i_moduleName) == 0)
            {
                for (u32 i = 0; i < i_module->numFunctions; ++i)
                {
                    IM3Function f = & i_module->functions [i];

                    if (f->name)
                    {
                        if (strcmp (f->name, i_functionName) == 0)
                        {
                            *o_function = f;
                            return 0;
                        }
                    }
                }
            }
        }
        
        i_module = i_module->next;
    }

    return -1;
}

const char* function_get_import_module(IM3Function i_function) {
    return i_function->import.moduleUtf8;
}

const char* function_get_import_field(IM3Function i_function) {
    return i_function->import.fieldUtf8;
}

int call(IM3Function i_function, uint32_t i_argc, int i_argv[]) {
	int result = 0;
	IM3Module module = i_function->module;
	IM3Runtime runtime = module->runtime;
	m3stack_t stack = (m3stack_t)(runtime->stack);
	IM3FuncType ftype = i_function->funcType;
	for (int i = 0; i < ftype->numArgs; i++) {
		int v = i_argv[i];
		m3stack_t s = &stack[i];
		*(u32*)(s) = v;
	}
	m3StackCheckInit();
	M3Result call_result = Call(i_function->compiled, stack, runtime->memory.mallocated, d_m3OpDefaultArgs);
	if(call_result != NULL) {
		set_error(call_result);
		return -1;
	}
	switch (ftype->returnType) {
		case c_m3Type_i32:
			result = *(u32*)(stack);
			break;
		case c_m3Type_i64:
		default:
			result =  *(u32*)(stack);
	};
	return result;
}

int get_allocated_memory_length(IM3Runtime i_runtime) {
	return i_runtime->memory.mallocated->length;
}

u8* get_allocated_memory(IM3Runtime i_runtime) {
	return m3MemData(i_runtime->memory.mallocated);
}

const void * native_dynamicFunctionWrapper(IM3Runtime runtime, uint64_t * _sp, void * _mem, uint64_t cookie) {
    int code = dynamicFunctionWrapper(runtime, _sp, _mem, cookie);
    return code == 0 ? m3Err_none : m3Err_trapExit;
}

int nextSlot = 0;
int attachFunction(IM3Module i_module, char* moduleName, char* functionName, char* signature) {
    int slot = nextSlot++;
    m3_LinkRawFunctionEx(i_module, moduleName, functionName, signature, native_dynamicFunctionWrapper, slot);
    return slot;
}
