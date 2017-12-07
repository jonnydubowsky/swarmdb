/*
   Copyright 2010-2016 Intel Corporation

   This software is licensed to you in accordance
   with the agreement between you and Intel Corporation.

   Alternatively, you can use this file in compliance
   with the Apache license, Version 2.


   Apache License, Version 2.0

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

#include "jhi_semaphore.h"

namespace intel_dal
{

	Semaphore::Semaphore(uint8_t semaphore_count)
	{
		win32semaphore = CreateSemaphore(NULL,semaphore_count,semaphore_count,NULL);
	}

	Semaphore::~Semaphore(void)
	{
		CloseHandle(win32semaphore);
	}

	void Semaphore::Acquire()
	{
		WaitForSingleObject(win32semaphore,INFINITE);
	}

	void Semaphore::Release()
	{
		ReleaseSemaphore(win32semaphore,1,NULL);
	}

}